package hl7_handlers

import (
	"fmt"
	"wisemed-labreaders/general"
	"wisemed-labreaders/protocols/hl7/hl7_segments"
	"wisemed-labreaders/sqlitewrapper"
)
import "github.com/lenaten/hl7"

// Handle the InventoryUpdateMessage (ESU^U01^ESU_U01) message came from the analyzers
// The message from the analyzer consists of 2 segments:
//
//	MSH
//	EQU
//
// Host should respond with HL7MessageTypes.HL7MsgType_InventoryUpdateMessageACK (ACK^U01^ACK)
func HandleHL7MsgType_InventoryUpdateMessage(msg *hl7.Message) ([]byte, error) {
	hasError := false

	parseErrLocation := hl7_segments.HL7ErrorWithLocation{}
	msh := hl7_segments.HL7MSH{}
	equ := hl7_segments.HL7EQU{}

	err := msg.Unmarshal(&msh)
	if err != nil {
		parseErrLocation.Location = "MSH^0^^^^"
		parseErrLocation.ErrorText = "An error has occured on unmarshaling MSH segment"
		hasError = true
	}

	if !hasError {
		err = msg.Unmarshal(&equ)
		if err != nil {
			parseErrLocation.Location = "EQU^0^^^^"
			parseErrLocation.ErrorText = "An error has occured on unmarshaling EQU segment"
			hasError = true
		}
	}

	var invSegments []*hl7.Segment
	if !hasError {
		invSegments, err = msg.AllSegments("INV")
		if err != nil {
			parseErrLocation.Location = "INV^0^^^^"
			parseErrLocation.ErrorText = "An error has occured on unmarshaling INV segment(s)"
			hasError = true
		}
	}

	if !hasError {
		//Now save them to the DB
		go saveTestDataToDB(invSegments)
	}

	ackMsg := ""
	if !hasError {
		ackMsg, err = buildACKResponse(msh, HL7MessageTypesDefinitions.HL7MsgType_InventoryUpdateMessageACK, "AA", nil)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
	} else {
		ackMsg, err = buildACKResponse(msh, HL7MessageTypesDefinitions.HL7MsgType_InventoryUpdateMessageACK, "AE", &parseErrLocation)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
	}

	return []byte(ackMsg), nil
}

func GetHL7MsgType_InventoryRequest() (string, error) {
	msg := hl7.NewMessage(nil)

	//SendStringQueue
	msh := hl7_segments.HL7MSH{}
	msh.CreateFromMessage(msg, HL7MessageTypesDefinitions.HL7MsgType_InventoryRequest, "", "")
	equ := hl7_segments.HL7EQU{}
	equ.CreateSegment()
	hl7Message, err := encodeHL7Objects(&msh, &equ)
	if err != nil {
		return "", err
	}

	return getHL7Packet(hl7Message), nil
	//HL7MsgType_InventoryRequest
}
func saveTestDataToDB(invSegments []*hl7.Segment) error {
	ktDB, err := sqlitewrapper.GetKnownTests()
	if err != nil {
		return err
	}
	knownTestIds := map[string]sqlitewrapper.SQLKnownTest{}
	for _, kt := range ktDB {
		knownTestIds[kt.Code] = kt
	}
	knownTests := general.ObjectQueue{}

	for _, inv := range invSegments {
		test, _ := inv.Get(&hl7.Location{Segment: "INV", FieldSeq: 1, Comp: 0})
		testStatus, _ := inv.Get(&hl7.Location{Segment: "INV", FieldSeq: 2, Comp: 0})
		testActive := 1
		if testStatus != "OK" {
			testActive = 0
		}
		tmpKt := sqlitewrapper.SQLKnownTest{Code: test, Active: testActive}

		if dbTest, ok := knownTestIds[test]; ok {
			//copy data not to rewrite it
			tmpKt.Copy(&dbTest)
			if tmpKt.Tag == "" {
				tmpKt.Tag = test
			}
			tmpKt.Active = testActive
		} else {
			tmpKt.Tag = test
		}
		knownTests.Push(tmpKt)
	}

	return sqlitewrapper.SaveKnownTestsBulk(&knownTests)
}
