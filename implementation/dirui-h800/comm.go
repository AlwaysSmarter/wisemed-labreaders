package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"wisemed-labreaders/config"
	"wisemed-labreaders/protocols"
	"wisemed-labreaders/protocols/stxetxpackages"
	"wisemed-labreaders/sqlitewrapper"
)

func createCommHandler() config.ProtocolHandler {
	return &stxetxpackages.STXETXProtocol{
		STXETXId:                time.Now().Format("20060102150405"),
		EventDataSent:           onDataSent,
		EventDataArrived:        onDataArrived,
		EventSetInitialized:     onSetInitialized,
		EventBroadcastWsMessage: onBroadcastWMMessage,

		CustomBlockDecoder: blockDecoder,
	}
}
func blockDecoder(proto *stxetxpackages.STXETXProtocol, ac config.AnalyzerConnection, data string) {
	dataArr := strings.Split(data, "\r\n")

	if len(dataArr) < 13 {
		proto.LogSTXETXMessage(fmt.Sprintf("Protocol error, recevied %d lines only!\nData: %s", len(dataArr), data), true, "logaerr")
	}

	tmpFile := sqlitewrapper.SQLOrder{Tests: []sqlitewrapper.SQLTest{}}
	if len(dataArr[0]) > 16 {
		dateStr := dataArr[0]
		if dateStr[3] == '-' && dateStr[5] == '-' {
			tmpFile.ResultReceivedDateTime = dateStr[0:15]
		}
	}
	lineFound, idx := findLineStartingWithText(1, "ID", dataArr)
	if !lineFound {
		proto.LogSTXETXMessage(fmt.Sprintf("\nProtocol error. ID not found %s\n", data), true, "logaerr")
		return
	}

	tmpFile.FileId = strings.TrimSpace(dataArr[idx][3:])
	lineFound, idx = findLineStartingWithText(idx+1, "PORT NO.", dataArr)
	if !lineFound {
		proto.LogSTXETXMessage(fmt.Sprintf("\nProtocol error. Port No. not found %s\n", data), true, "logaerr")
		return
	}
	idx++
	anRowRegExp := regexp.MustCompile(`([0-9A-Za-z]+)([\ ]+)([a-zA-Z0-9\+\-\\\/\=\>\<\.\,]+)(\ )*([[:ascii:]]*)`)
	//will have - matchedTXT anTxt spaceTXT resTXT [spaceTXT interpretation]
	for len(dataArr) > idx {
		tmpStr := strings.TrimSpace(dataArr[idx])
		if tmpStr == "" {
			idx++
			continue
		}

		regExpRes := anRowRegExp.FindStringSubmatch(dataArr[idx])
		if len(regExpRes) < 4 {
			idx++
			continue
		}
		tstCode := regExpRes[1]
		tstResult := regExpRes[3]
		tstInterpretation := ""
		if len(regExpRes) > 5 {
			tstInterpretation = regExpRes[5]
		}

		tmpFile.Tests = append(tmpFile.Tests, sqlitewrapper.SQLTest{Code: tstCode, Raw: tstResult, Interpretation: tstInterpretation})

		idx++
	}
	proto.ResultsFilesQueue.Push(tmpFile)
	go proto.SaveResultToDatabase()
}

func findLineStartingWithText(fromIdx int, search string, dataArr []string) (bool, int) {
	idx := 1
	for len(dataArr) > idx {
		tmpStr := strings.TrimSpace(dataArr[idx])
		if strings.HasPrefix(tmpStr, search) {
			break
		}
		idx++
	}
	return len(dataArr) > idx, idx
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
