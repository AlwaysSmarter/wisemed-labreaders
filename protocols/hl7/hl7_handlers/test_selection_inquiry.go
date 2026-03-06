package hl7_handlers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lenaten/hl7"
	"wisemed-labreaders/protocols/hl7/hl7_segments"
	"wisemed-labreaders/sqlitewrapper"
	"wisemed-labreaders/wisemed"
)

var ForceFilterBySampleType = true
var sentOrderTests = map[string]string{}

// Handle the TestSelectionInquiry (QBP^Q11^QBP_Q11) message came from the analyzers
// The message from the analyzer consists of 2 segments:
//
//	MSH
//	QPD
//	RCP
//
// Example:
//
//	"\vMSH|^~\\&|cobas pro||Host||20230128204627+0900||QBP^Q11^QBP_Q11|159|P|2.5.1|||NE|AL||UNICODE UTF-8|||LAB-27R^ROCHE\rQPD|INIBAR^^99ROC|262|498100|50006|1|||||SERPLAS^^99ROC|SC^^99ROC|R\rRCP|I|1|R^HL70394\r\x1c\r"
//
// Host should respond with HL7MessageTypes.HL7MsgType_ResponseMessageFromHost (RSP^K11^RSP_K11)
func HandleHL7MsgType_TestSelectionInquiry(msg *hl7.Message, msgChan chan string, stopChan chan bool, errChan chan error) {
	var err error
	hasError := false

	parseErrLocation := hl7_segments.HL7ErrorWithLocation{}
	msh := hl7_segments.HL7MSH{}
	qpd := hl7_segments.HL7QPD{}
	rcp := hl7_segments.HL7RCP{}

	err = msg.Unmarshal(&msh)
	if err != nil {
		parseErrLocation.Location = "MSH^0^^^^"
		parseErrLocation.ErrorText = "An error has occured on unmarshaling MSH segment"
		hasError = true
	}

	if !hasError {
		err = msg.Unmarshal(&qpd)
		if err != nil {
			parseErrLocation.Location = "QPD^0^^^^"
			parseErrLocation.ErrorText = "An error has occured on unmarshaling QPD segment"
			hasError = true
		}
	}
	if !hasError {
		err = msg.Unmarshal(&rcp)
		if err != nil {
			parseErrLocation.Location = "RCP^0^^^^"
			parseErrLocation.ErrorText = "An error has occured on unmarshaling RCP segment"
			hasError = true
		}
	}

	fileId := 0

	if fileId, err = strconv.Atoi(qpd.ContainerId); err != nil {
		fmt.Printf("\nReceived %q fileId is not an integer number\nError: %s", qpd.ContainerId, err)
		errChan <- err
	}
	if fileId <= 0 {
		errText := "Fisa ID trebuie sa fie un numai mai mare decat 0"
		fmt.Printf("\n%s\n%q", errText, err)
		errChan <- errors.New(errText)
	}
	posNo := 0
	if posNo, err = strconv.Atoi(qpd.PositionNo); err != nil {
		fmt.Printf("\nReceived %q position no is not an integer number\nError: %s", qpd.PositionNo, err)
		errChan <- err
	}
	if posNo <= 0 {
		errText := "Pozitia trebuie sa fie un numai mai mare decat 0"
		fmt.Printf("\n%s\n%q", errText, err)
		errChan <- errors.New(errText)
	}

	orderChan := make(chan *sqlitewrapper.SQLOrder)
	errChanLocal := make(chan error)
	go func() {
		//Now save them to the DB
		nowt := time.Now()
		orderRec, _, err := wisemed.LoadFileFromWMAsObj(nowt.Format("2006-01-02"), -1, -1, posNo, fileId)
		if err != nil {
			errChanLocal <- err
			return
		}
		orderChan <- orderRec

	}()

	select {
	case err := <-errChanLocal:
		errChan <- err
		return
	case orderRec := <-orderChan:
		fmt.Printf("\nLoaded file from WM: %v\n", orderRec)

		testsChan := make(chan []sqlitewrapper.SQLTest)
		go func(orderRec *sqlitewrapper.SQLOrder) {
			neededTests, err := returnNeededTestsForOrder(qpd.SampleType, orderRec)

			if err != nil {
				errChanLocal <- err
				return
			}

			testsChan <- neededTests

		}(orderRec)

		select {
		case err := <-errChanLocal:
			errChan <- err
			return
		case neededTests := <-testsChan:
			fmt.Printf("\nData fully loaded, tests available: %v\n", neededTests)
			if debugHL7Handlers > 2 {
				fmt.Println("NeededTests for order:")
				fmt.Println(neededTests)
			}

			//we launch it here and wait for them to wait in the main hl7_interface call
			go buildResponseMessageFromHost(msh, qpd, "AA", orderRec, neededTests, false, false, nil, msgChan, stopChan, errChan)
			return
		}
	}
}

func returnNeededTestsForOrder(sampleType string, order *sqlitewrapper.SQLOrder) ([]sqlitewrapper.SQLTest, error) {
	if debugHL7Handlers > 10 {
		fmt.Printf("\nFunction needed tests for order\n")
		fmt.Printf("\nSample type %s\n", returnNeededTestsForOrder)
	}
	sampleTypeArr := strings.Split(sampleType, "^")
	sampleType = sampleTypeArr[0]
	ktDB, err := sqlitewrapper.GetKnownTests()
	if err != nil {
		return nil, err
	}
	resp := []sqlitewrapper.SQLTest{}

	availableTestsForTheSampleType := map[string]sqlitewrapper.SQLKnownTest{}

	for _, kt := range ktDB {

		//fmt.Println(kt)
		if kt.Active != 1 {
			//skip inactive tests
			continue
		}
		if debugHL7Handlers > 10 {
			fmt.Printf("\nParsing KT %s - %s - %s", kt.Tag, kt.Code, kt.Details)
		}
		if kt.Details != "" || ForceFilterBySampleType {
			props := strings.Split(kt.Details, ";")
			foundProp := ""

			if debugHL7Handlers > 10 {
				fmt.Println("Properties of KT")
				fmt.Println(props)
			}
			for _, prop := range props {
				keyVal := strings.Split(prop, "=")
				if len(keyVal) != 2 {
					if debugHL7Handlers > 10 {
						fmt.Println("Skip - not a property")
					}
					continue
				}
				if keyVal[0] == "restrict_to_sample_type" {
					if debugHL7Handlers > 10 {
						fmt.Println("Found restrict_to_sample_type property")
					}
					foundProp = keyVal[1]
					break
				}
			}

			if foundProp == "" && ForceFilterBySampleType {
				if debugHL7Handlers > 10 {
					fmt.Println("Skip,  property restrict_to_sample_type not found")
				}
				continue
			}
			if strings.ToLower(foundProp) != strings.ToLower(sampleType) {
				if debugHL7Handlers > 10 {
					fmt.Printf("Skip,  property %s not %s \n", foundProp, sampleType)
				}
				continue
			}

		}

		availableTestsForTheSampleType[kt.Tag] = kt
	}
	nowt := time.Now()

	for _, order := range order.Tests {
		if order.Tag == "" {
			continue
		}
		if _, ok := availableTestsForTheSampleType[order.Tag]; ok {
			resp = append(resp, order)

			//try to save to DB the fac that was ordered

			tstHistoryQuery := sqlitewrapper.SQLTestHistoryQuery{Id: "0"}
			go func() {
				tstHistory := sqlitewrapper.SQLTestHistory{
					TestId: order.Id,
					SentOn: nowt.Format("20060102150405-0700"),
					Status: "S",
					SentBy: "", //TODO
				}
				sqlitewrapper.SaveTestHistory(tstHistoryQuery, tstHistory)
			}()
		}
	}
	return resp, nil
}

func sengMessageToChannel(obj interface{}, msgChan chan<- string, errChan chan<- error) bool {
	hl7Message, err := encodeHL7Objects(obj)
	if err != nil {
		errChan <- err
		return false
	}
	msgChan <- string(hl7Message.Value) + string(CR)
	return true
}

// msh
// msa
// [err]
// qak
// qpd
func buildResponseMessageFromHost(msh hl7_segments.HL7MSH, qpd hl7_segments.HL7QPD, ackCode string, order *sqlitewrapper.SQLOrder, tests []sqlitewrapper.SQLTest, isEmergency bool, isSeqMode bool, errWithLoc *hl7_segments.HL7ErrorWithLocation, msgChan chan<- string, stopChan chan<- bool, errChan chan<- error) {
	var err error

	//Send the package to analyzer as I am constructing it

	//The package is <VT><MSHSeg>\r<...other segments spluted by \r>\r<VT>\r
	msgChan <- string(VT)
	mshResp := msh.CopyForResponse(HL7MessageTypesDefinitions.HL7MsgType_ResponseMessageFromHost)
	mshResp.AcceptAcknowledgementType = ""
	mshResp.ApplicationAcknowledgementType = ""
	mshResp.MessageProfileID = "LAB-27R^ROCHE"

	originalMessageControl := mshResp.MessageControl
	msgControlInt, err := strconv.Atoi(originalMessageControl)
	if err != nil {
		msgControlInt = int(time.Now().UnixMilli())
	}
	//msgControlInt++
	mshResp.MessageControl = fmt.Sprintf("%d", msgControlInt)

	if !sengMessageToChannel(&mshResp, msgChan, errChan) {
		return
	}

	msaResp := hl7_segments.HL7MSA{}
	msaResp.CreateSegment(ackCode, originalMessageControl)

	if !sengMessageToChannel(&msaResp, msgChan, errChan) {
		return
	}

	if ackCode != "AA" {
		//create the error block too
		errResp := hl7_segments.HL7ERR{}
		errResp.CreateSegment(errWithLoc.Location, errWithLoc.ErrorId, errWithLoc.ErrorText)

		if !sengMessageToChannel(&errResp, msgChan, errChan) {
			return
		}
	}

	qakResp := hl7_segments.HL7QAK{}
	qakResp.CopyFromQPD(qpd, "OK")

	if !sengMessageToChannel(&qakResp, msgChan, errChan) {
		return
	}

	qpdResp := hl7_segments.HL7QPD{}
	qpdResp.CreateSegmentForResultOrderQuery(qpd.QueryTag, qpd.ContainerId, qpd.SampleTypeId, qpd.SampleTypeTxt, qpd.SampleTypeCoding, qpd.SampleContainerTypeId, qpd.SampleContainerTypeTxt, qpd.SampleContainerTypeCoding)
	qpdResp.MessageNameId = qpd.MessageNameId
	qpdResp.MessageNameTest = qpd.MessageNameTest
	qpdResp.MessageNameCodingSystem = qpd.MessageNameCodingSystem
	qpdResp.RackId = qpd.RackId
	qpdResp.PositionNo = qpd.PositionNo
	if isEmergency {
		qpdResp.Priority = "S"
	} else {
		qpdResp.Priority = "R"
	}
	if !sengMessageToChannel(&qpdResp, msgChan, errChan) {
		return
	}

	//send END of PACKAGE
	msgChan <- fmt.Sprintf("%s%s", string(FS), string(CR))

	//send START of PACKAGE
	msgChan <- string(VT)

	mshResp.MessageType = HL7MessageTypesDefinitions.HL7MsgType_TestSelectionInformationReceive
	//msgControlInt++
	mshResp.MessageControl = fmt.Sprintf("%d", msgControlInt)
	mshResp.AcceptAcknowledgementType = "NE"
	mshResp.ApplicationAcknowledgementType = "AL"
	mshResp.MessageProfileID = "LAB-28R^ROCHE"

	if !sengMessageToChannel(&mshResp, msgChan, errChan) {
		return
	}

	if isSeqMode {
		stopChan <- true
	}

	pid := hl7_segments.HL7PID{}
	pid.CreateSegment()
	pid.PatientId = order.FileId
	pid.Unused5 = order.PatientName
	pid.PatientNameTypeCode = "U"
	//pid.Birthdate = order.
	if !sengMessageToChannel(&pid, msgChan, errChan) {
		return
	}

	spm := hl7_segments.HL7SPM{}
	spm.CreateSegment()
	spm.SampleInformation = fmt.Sprintf("%s&BARCODE", order.FileId)
	spm.SampleSeqId = order.FileId
	spm.SampleType = "BARCODE"
	spm.SpecimenType = qpd.SampleType
	spm.SpecimenIdentifier = qpd.SampleTypeId
	spm.SpecimenText = qpd.SampleTypeTxt
	spm.SpecimenCodingSystem = qpd.SampleTypeCoding
	spm.SpecimenRoleId = "P"
	spm.SpecimenRoleCodingSystem = "HL70369"
	spm.Comment = "~~~~"
	spm.SepcimenCollectionDatetime = time.Now().Format("20060102") + "080000"
	spm.ContainerType = qpd.SampleContainerType
	if !sengMessageToChannel(&spm, msgChan, errChan) {
		return
	}

	sac := hl7_segments.HL7SAC{}
	sac.CreateSegment()
	//sac.SampleInformation = fmt.Sprintf("%s^BARCODE", order.FileId)
	sac.SampleSeqId = order.FileId
	sac.SampleType = "BARCODE"
	sac.RackId = qpd.RackId
	sac.PositionNo = qpd.PositionNo
	sac.PreDilutionCode = "^1^:^1"
	if !sengMessageToChannel(&sac, msgChan, errChan) {
		return
	}

	obrSetID := 1

	for _, test := range tests {
		orc := hl7_segments.HL7ORC{}
		orc.CreateSegment(true)
		orc.OrderControl = "NW" //NW - new; CA - order cancelation; DC - discontinue

		if !sengMessageToChannel(&orc, msgChan, errChan) {
			return
		}

		tq1 := hl7_segments.HL7TQ1{}
		tq1.CreateSegment()

		if isEmergency {
			tq1.PriorityId = "S"
			tq1.PriorityCoding = "HL70485"
		} else {
			tq1.PriorityId = "R"
			tq1.PriorityCoding = "HL70485"
		}
		if !sengMessageToChannel(&tq1, msgChan, errChan) {
			return
		}

		obr := hl7_segments.HL7OBRHOST{}
		obr.CreateSegment()
		obr.SetID = strconv.Itoa(obrSetID)
		obrSetID++

		obr.PlacerOrderNumber_EntityId = order.FileId

		obr.USIId = test.Code
		obr.USICodingSystem = "99ROC"
		obr.SpecimenActionCode = ""
		//obr.SICalibrationMethod = "Full"
		//obr.SICodingSystem = "99ROC"
		if !sengMessageToChannel(&obr, msgChan, errChan) {
			return
		}

		tcd := hl7_segments.HL7TCDHOST{}
		tcd.CreateSegment()
		tcd.USIId = test.Code
		tcd.USICodingSystem = "99ROC"
		tcd.AutoDilutionFactor = "" //"1^:^1"
		if !sengMessageToChannel(&tcd, msgChan, errChan) {
			return
		}
	}

	//Send END PACKAGE
	msgChan <- fmt.Sprintf("%s%s", string(FS), string(CR))

	//Stop communication
	stopChan <- true
}

// Handle the TestSelectionInquiryACK (ORL^O34^ORL_O42) message came from the analyzers in response to a TestSelectionInquiry response
// The message from the analyzer consists of teh following segments:
//
//		MSH
//		MSA
//		[ERR]
//		[PID]
//		SPM
//		SAC
//	 {ORC}
//
// Example:
//
//	"\vMSH|^~\\&|cobas pro||Host||20230128204627+0900||ORL^O34^ORL_O42|160|P|2.5.1||||||UNICODE UTF-8\rMSA|AA|159\rPID|||498100||Popescu Marius^^^^^^U|||\rSPM|1|498100&BARCODE||SERPLAS^^99ROC|||||||P^^HL70369|||~~~~|||||||||||||SC^^99ROC\rSAC|||498100&BARCODE|||||||50006|1||||||||||||||||||\rORC|OK||||SC\rORC|OK||||SC\rORC|OK||||SC\rORC|OK||||SC\rORC|OK||||SC\r\x1c\r"
//
// Host should not respond
func HandleHL7MsgType_TestSelectionInquiryACK(msg *hl7.Message) error {
	//sentOrderTests

	msh := hl7_segments.HL7MSH{}
	err := msg.Unmarshal(&msh)
	if err != nil {
		fmt.Println("An error has occured on unmarshaling MSH segment")
		return err
	}

	msa := hl7_segments.HL7MSA{}
	err = msg.Unmarshal(&msa)
	if err != nil {
		fmt.Println("An error has occured on unmarshaling MSA segment")
		return err
	}

	errSeg := hl7_segments.HL7ERR{}
	err = msg.Unmarshal(&errSeg)
	if err != nil {
		fmt.Println("An error has occured on unmarshaling ERR segment")
		return err
	}

	pid := hl7_segments.HL7PID{}
	err = msg.Unmarshal(&pid)
	if err != nil {
		fmt.Println("An error has occured on unmarshaling PID segment")
		return err
	}

	spm := hl7_segments.HL7SPM{}
	err = msg.Unmarshal(&spm)
	if err != nil {
		fmt.Println("An error has occured on unmarshaling SPM segment")
		return err
	}

	if spm.AssignedSpecimenId == "" {
		return nil
	}

	tmpArr := strings.Split(spm.SampleSeqId, "&")
	fileId := tmpArr[0]
	if _, ok := sentOrderTests[fileId]; !ok {
		return nil
	}

	sac := hl7_segments.HL7SAC{}
	err = msg.Unmarshal(&sac)
	if err != nil {
		fmt.Println("An error has occured on unmarshaling SAC segment")
		return err

	}

	var orcSegments []*hl7.Segment
	orcSegments, err = msg.AllSegments("ORC")
	if err != nil {
		fmt.Println("An error has occured on unmarshaling ORC segment(s)")
		return err
	}

	savedTestHistroyIds := strings.Split(sentOrderTests[fileId], ",")

	for thIdx, thId := range savedTestHistroyIds {
		if thIdx >= len(orcSegments) {
			break
		}
		orcSeg := orcSegments[thIdx]
		isOK, _ := orcSeg.Get(&hl7.Location{Segment: "ORC", FieldSeq: 1, Comp: 0})

		thIdInt, err := strconv.Atoi(thId)
		if err != nil {
			continue
		}

		thDB, err := sqlitewrapper.GetTestHistory(thIdInt)
		if err != nil {
			return err
		}
		if isOK == "OK" {
			thDB.Status = "C"
		} else {
			thDB.Status = "E"
		}

		thQuery := sqlitewrapper.SQLTestHistoryQuery{Id: thId}
		_, _, err = sqlitewrapper.SaveTestHistory(thQuery, thDB)
		if err != nil {
			return err
		}
	}

	delete(sentOrderTests, spm.AssignedSpecimenId)
	return nil
}
