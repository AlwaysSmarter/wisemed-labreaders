package main

import (
	"fmt"
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
		ASTMId:                  time.Now().Format("20060102150405"),
		EventDataSent:           onDataSent,
		EventDataArrived:        onDataArrived,
		EventSetInitialized:     onSetInitialized,
		EventBroadcastWsMessage: onBroadcastWMMessage,

		CFGBlockUnknownSerial: "",

		CustomReceiveResultSegment: ReceiveResultSegment,
		CustomBuildHeaderRecord:    BuildHeaderRecord,
		CustomBuildPatientRecord:   BuildPatientRecord,
		CustomBuildTestOrderRecord: BuildTestOrderRecord,
		//CustomBuildTestResultRecord        func(proto *ASTMProtocol, ASTMRec ASTMResRecSegment) string

	}
}

func BuildHeaderRecord(proto *astm.ASTMProtocol, ASTMRec astm.ASTMHeaderSegment) string {
	ASTMRec.MessageControlID = ""
	ASTMRec.AccessPassword = ""
	ASTMRec.SenderStrAddr = ""
	ASTMRec.ReservedField = ""
	ASTMRec.SenderPhoneNo = ""
	ASTMRec.SenderCharacteristics = ""
	ASTMRec.CommentSI = ""
	ASTMRec.Processing = ""
	ASTMRec.ASTMVer = "1"
	ASTMRec.DateAndTime = ""

	if ASTMRec.SenderName == "" {
		ASTMRec.SenderName = "HostName^^^^"
	}
	analyzerArr := strings.Split(ASTMRec.ReceiverID, "^")
	if len(analyzerArr) > 0 {
		ASTMRec.ReceiverID = analyzerArr[0]
	}

	return ASTMRec.GetASTMSegment(9)
}

func BuildPatientRecord(proto *astm.ASTMProtocol, ASTMRec astm.ASTMPIDSegment) string {
	ASTMRec.SequenceNo = "1"
	return ASTMRec.GetASTMSegment(1)
}

func BuildTestOrderRecord(proto *astm.ASTMProtocol, ASTMRecs []astm.ASTMTORSegment) []string {
	//analyzerArr := strings.Split(ASTMRec.SampleID, "^")
	//if len(analyzerArr) > 0 {
	//	ASTMRec.ReceiverID = analyzerArr[0]
	//}
	rec := []string{}

	if len(ASTMRecs) <= 0 {
		return rec
	}
	analisysCodes := ""
	var sendASTMRec *astm.ASTMTORSegment

	parsedCodes := map[string]int{}
	for _, ASTMRec := range ASTMRecs {
		_, ok := parsedCodes[ASTMRec.UniversalTestID]
		if ok {
			continue
		} else {
			parsedCodes[ASTMRec.UniversalTestID] = 1
		}

		if sendASTMRec == nil {
			sendASTMRec = &ASTMRec
			sendASTMRec.InstrSpecimenID = ""
			if sendASTMRec.SequenceNo == "" {
				sendASTMRec.SequenceNo = "1"
			}
			if sendASTMRec.Priority == "" {
				sendASTMRec.Priority = "R"
			}
			if sendASTMRec.RequestedDateTime == "" {
				nowt := time.Now()
				sendASTMRec.RequestedDateTime = nowt.Format("20060102150405")
			}
		}

		if len(ASTMRec.UniversalTestID) > 3 {
			ASTMRec.UniversalTestID = ASTMRec.UniversalTestID[:3]
		}
		if len(ASTMRec.UniversalTestID) < 3 {
			ASTMRec.UniversalTestID = fmt.Sprintf("%03s", ASTMRec.UniversalTestID)
		}

		ASTMRec.UniversalTestID = fmt.Sprintf("%s0", ASTMRec.UniversalTestID[:2])

		if analisysCodes != "" {
			analisysCodes += "\\"
		}

		analisysCodes = fmt.Sprintf("%s^^^%s^^100", analisysCodes, ASTMRec.UniversalTestID)
	}
	sendASTMRec.UniversalTestID = analisysCodes
	rec = append(rec, sendASTMRec.GetASTMSegment(11))

	return rec
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

func ReceiveResultSegment(proto *astm.ASTMProtocol, ASTMRec *astm.ASTMResRecSegment) error {
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

	return nil
}
