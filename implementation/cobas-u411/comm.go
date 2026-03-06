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
)

func createCommHandler() config.ProtocolHandler {
	return &astm.ASTMProtocol{
		ASTMId:                     time.Now().Format("20060102150405"),
		EventDataSent:              onDataSent,
		EventDataArrived:           onDataArrived,
		EventSetInitialized:        onSetInitialized,
		EventBroadcastWsMessage:    onBroadcastWMMessage,
		CustomReceiveOrderSegment:  astmPlusReceiveOrderSegment,
		CustomReceiveResultSegment: astmPlusCustomReceiveResultSegment,
		CFGBlockUnknownSerial:      "",
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

func astmPlusCustomReceiveResultSegment(proto *astm.ASTMProtocol, ASTMRec *astm.ASTMResRecSegment) error {
	if proto.FileData.FileId == "" || proto.FileData.FileId == "0" {
		return nil
	}
	strLines := general.StringQueue{}
	strLines.Split(ASTMRec.UniversalTestID, "^", true)

	testCode := strings.Replace(ASTMRec.UniversalTestID, "^", "", -1)

	if strLines.Len() > 1 {
		testCode = strLines.GetStringOrVoid(0)
	}

	testCode = strings.TrimSpace(testCode)
	tmpAn := sqlitewrapper.SQLTest{Code: testCode, Raw: strings.TrimSpace(ASTMRec.DataValue)}
	proto.FileData.Tests = append(proto.FileData.Tests, tmpAn)

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

		if strLines.Len() >= 5 {
			if strings.Contains(strLines.GetStringOrVoid(4), "CONTROL") {
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
