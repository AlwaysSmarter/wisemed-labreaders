package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
	"wisemed-labreaders/protocols"
	"wisemed-labreaders/protocols/astm"
	"wisemed-labreaders/sqlitewrapper"
	"wisemed-labreaders/wisemed"
)

func createCommHandler() config.ProtocolHandler {
	return &astm.ASTMProtocol{
		ASTMId:                          time.Now().Format("20060102150405"),
		EventDataSent:                   onDataSent,
		EventDataArrived:                onDataArrived,
		EventSetInitialized:             onSetInitialized,
		EventBroadcastWsMessage:         onBroadcastWMMessage,
		CustomReceiveOrderSegment:       astmPlusReceiveOrderSegment,
		CustomReceiveResultSegment:      astmPlusCustomReceiveResultSegment,
		CustomBlockLinesSplitter:        astmLinesSplitter,
		CustomReceiveQuerySegment:       astmReceiveQuerySegment,
		CustomEachLineWithoutSTXETX:     true,
		CustomEachLineWithoutSequenceNo: true,
		CFGBlockUnknownSerial:           "",
	}
}

func onBroadcastWMMessage(msgType string, data string) {
	msg := fmt.Sprintf("\n<===(bm) %q (%d chars)\n", data, protocols.StrLen(data))
	fmt.Print(msg)
	if mySrv != nil {
		msg := map[string]interface{}{"success": true, "action": msgType, "msg": msg}
		mySrv.BroadcastWMMessage(msg)
	}
}

func onDataArrived(data string) {
	fmt.Printf("\n===>(da) %q (%d chars)\n", data, protocols.StrLen(data))
	if mySrv != nil {
		msg := map[string]interface{}{"success": true, "action": "analyzermsg", "msg": data}
		mySrv.BroadcastWMMessage(msg)
	}

}

func onDataSent(data string) {
	fmt.Printf("\n<===(ds) %q (%d chars)\n", data, protocols.StrLen(data))
	if mySrv != nil {
		msg := map[string]interface{}{"success": true, "action": "hostmsg", "msg": data}
		mySrv.BroadcastWMMessage(msg)
	}

}
func onSetInitialized(initialized bool) {
	//fmt.Println("INIT STATE %v", initialized)
}

func astmLinesSplitter() string {
	return "\r"
}
func astmPlusCustomReceiveResultSegment(proto *astm.ASTMProtocol, ASTMRec *astm.ASTMResRecSegment) error {
	if proto.FileData.FileId == "" || proto.FileData.FileId == "0" {
		return nil
	}
	strLines := general.StringQueue{}
	strLines.Split(ASTMRec.UniversalTestID, "^", true)

	testCode := strings.Replace(ASTMRec.UniversalTestID, "^", "", -1)

	if strLines.Len() > 3 {
		testCode = strLines.GetStringOrVoid(3)
	}

	testCode = strings.TrimSpace(testCode)
	tmpAn := sqlitewrapper.SQLTest{Code: testCode, Raw: strings.TrimSpace(ASTMRec.DataValue)}
	proto.FileData.Tests = append(proto.FileData.Tests, tmpAn)

	fmt.Printf("\nFound test: %s %q", tmpAn.Code, tmpAn.Raw)
	return nil
}

func astmPlusReceiveOrderSegment(proto *astm.ASTMProtocol, ASTMRec *astm.ASTMTORSegment) error {
	fmt.Printf("\n\nCallig astmPlusReceiveOrderSegment\n\n")
	if proto.FileData.FileId != "0" && proto.FileData.FileId != "" {
		return nil
	}

	if len(proto.CFGFileIDParser) <= 0 {
		proto.CFGFileIDParser = astm.DefaultFileIDParser
	}

	for _, parser := range proto.CFGFileIDParser {
		tmpVal := parser.TryToParse(ASTMRec)
		tmpValInt, ok := tmpVal.(int)
		if ok && tmpValInt > 0 {
			proto.FileData.FileId = strconv.Itoa(tmpValInt)
			break
		}
	}

	fmt.Printf("\nI have the following FILEID: %q", proto.FileData.FileId)
	if proto.CustomISQcDecoder != nil {
		proto.FileData.ResultType = proto.CustomISQcDecoder(proto, ASTMRec)
	} else {
		strLines := general.StringQueue{}
		strLines.Split(ASTMRec.InstrSpecimenID, "^", true)

		if strLines.Len() >= 4 {
			if strings.Contains(strLines.GetStringOrVoid(2), "CONTROL") {
				proto.FileData.ResultType = sqlitewrapper.ResultQC
			} else {
				if strings.TrimSpace(ASTMRec.ActionCode) == "Q" {
					proto.FileData.ResultType = sqlitewrapper.ResultQC
				}
			}
		}
	}
	return nil
}

func astmReceiveQuerySegment(proto *astm.ASTMProtocol, ASTMRec *astm.ASTMRIRSegment) error {
	go func() {
		if proto.HasValidOrder() {
			proto.OrderReqestsQueue.Push(proto.OrderData)
		}
		proto.OrderData = sqlitewrapper.SQLOrder{} //ready for a new one

		strLines := general.StringQueue{}
		strLines.Split(ASTMRec.StartingRangeID, "^", true) //should have smth like 000001^01^         619138^B
		if strLines.Len() > 0 {
			proto.OrderData.FileId = strings.TrimSpace(strLines.GetStringOrVoid(1))
			proto.OrderData.Tests = []sqlitewrapper.SQLTest{}

			fileId, err := strconv.Atoi(proto.OrderData.FileId)
			if err != nil || fileId <= 0 {
				proto.LogASTMMessage(fmt.Sprintf("\nInvalid File ID %s from segment %s", proto.OrderData.FileId, ASTMRec.StartingRangeID), true, "logaerr")
				return
			}
			nowt := time.Now()
			//first load order from DB

			readerOrderSQLITE, _, err := wisemed.LoadFileFromWMAsObj(nowt.Format("2006-01-02"), -1, -1, -1, fileId)
			if err != nil {
				proto.LogASTMMessage(fmt.Sprintf("An error has occured!\n%v", err), true, "logaerr")
				return
			}
			readerOrderSQLITE.FormatDatesForDB()
			proto.LogASTMMessage(fmt.Sprintf("\nFile loaded from WM:\n%d name: %s", readerOrderSQLITE.FileId, readerOrderSQLITE.PatientName), true, "logamsg")
			proto.LogASTMMessage("\nTests on file:", true, "logamsg")
			//creating a package to send to the analyzer
			for tstIdx, tst := range readerOrderSQLITE.Tests {
				proto.LogASTMMessage(fmt.Sprintf("\n%d - %s [%s] (%s);", tstIdx, tst.Name, tst.Code, tst.Tag), true, "logamsg")
				proto.OrderData.Tests = append(proto.OrderData.Tests, tst)
			}

		}

		proto.OrderReqestsQueue.Push(proto.OrderData)
		proto.OrderData = sqlitewrapper.SQLOrder{}
		proto.OrderData.Tests = nil
		go proto.ParseOrderSegmentsToASTMPackages()
	}()

	return nil
}
