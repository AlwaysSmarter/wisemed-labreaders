package main

import (
	"fmt"
	"strings"
	"wisemed-labreaders/config"
	"wisemed-labreaders/general"
	"wisemed-labreaders/protocols"
	"wisemed-labreaders/protocols/astm"
	"wisemed-labreaders/sqlitewrapper"
)

func createCommHandler() config.ProtocolHandler {
	return &astm.ASTMProtocol{
		EventDataArrived:    onDataArrived,
		EventSetInitialized: onSetInitialized,

		CFGBlockUnknownSerial: "16839",
	}
}

func onDataArrived(data string) {
	fmt.Printf("===> [%v] %q (%d chars)\n", protocols.RuneAt(data, 0), data, protocols.StrLen(data))
}
func onSetInitialized(initialized bool) {
	//fmt.Println("INIT STATE %v", initialized)
}

func CustomResultDecoder(proto *astm.ASTMProtocol, ASTMRec *astm.ASTMResRecSegment) {
	if proto.FileData.FileId <= 0 {
		return
	}
	strLines := general.StringQueue{}
	strLines.Split(ASTMRec.UniversalTestID, "^", true)

	testName := strings.TrimSpace(strings.Replace(ASTMRec.UniversalTestID, "^", "", -1))
	testValue := ASTMRec.DataValue
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

}
