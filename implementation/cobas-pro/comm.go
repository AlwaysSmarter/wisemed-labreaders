package main

import (
	"fmt"
	"time"
	"wisemed-labreaders/config"
	"wisemed-labreaders/protocols"
	"wisemed-labreaders/protocols/hl7"
)

func createCommHandler() config.ProtocolHandler {
	return &hl7.HL7Protocol{
		HL7Id:                   time.Now().Format("20060102150405"),
		EventDataSent:           onDataSent,
		EventDataArrived:        onDataArrived,
		EventSetInitialized:     onSetInitialized,
		EventBroadcastWsMessage: onBroadcastWMMessage,

		CFGBlockUnknownSerial: "16839",
	}
}

func onBroadcastWMMessage(msgType string, data string) {
	fmt.Printf("\n<===(bm) %q (%d chars)\n", data, protocols.StrLen(data))
	if mySrv != nil {
		msg := map[string]interface{}{"success": true, "action": msgType, "msg": data}
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

func CustomResultDecoder(proto *hl7.HL7Protocol, HL7Rec interface{}) {
	/*
		if proto.FileData.FileId <= 0 {
			return
		}
		strLines := general.StringQueue{}
		strLines.Split(HL7Rec.UniversalTestID, "^", true)

		testName := strings.TrimSpace(strings.Replace(HL7Rec.UniversalTestID, "^", "", -1))
		testValue := HL7Rec.DataValue
		if strLines.Len() > 4 {
			testName = strLines.GetStringOrVoid(4)
		} else {
			if strLines.Len() > 3 {
				testName = strLines.GetStringOrVoid(3)
			}
		}
		switch testName {
		case "DIST_RBC":
			testValue = strings.Replace(testValue, "&R&", "\\", -1)
			//DecodePngGraph(FPNGPath + '\' + test_value, myPatient.RBCGraph);
			break
		case "DIST_PLT":
			testValue = strings.Replace(testValue, "&R&", "\\", -1)
			//DecodePngGraph(FPNGPath + '\' + test_value, myPatient.PLTGraph);
			break
		}

		tmpRes := sqlitewrapper.SQLTestResult{Result: testValue}
		tmpAn := sqlitewrapper.SQLTest{Name: testName, Code: testName, Result: tmpRes.Result}
		proto.FileData.Tests = append(proto.FileData.Tests, tmpAn)
	*/
}
