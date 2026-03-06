package hl7_handlers

import (
	"fmt"
	"wisemed-labreaders/protocols/hl7/hl7_segments"
)
import "github.com/lenaten/hl7"

// Handle the EquipmentStatusMessage (ESU^U01^ESU_U01) message came from the analyzers
// The message from the analyzer consists of 2 segments:
//
//	MSH
//	EQU
//
// Host should respond with HL7MessageTypes.HL7MsgType_EquipmentStatusMessageACK (ACK^U01^ACK)
func HandleHL7MsgType_EquipmentStatusMessage(msg *hl7.Message) ([]byte, error) {
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

	if !hasError {
		err, pel := verifySegmentValidity(msg, &msh)
		if err != nil {
			parseErrLocation = *pel
			hasError = true
		}
	}

	if !hasError {
		err, pel := verifySegmentValidity(msg, &equ)
		if err != nil {
			parseErrLocation = *pel
			hasError = true
		}
	}

	ackMsg := ""
	if !hasError {
		ackMsg, err = buildACKResponse(msh, HL7MessageTypesDefinitions.HL7MsgType_EquipmentStatusMessageACK, "AA", nil)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
	} else {
		ackMsg, err = buildACKResponse(msh, HL7MessageTypesDefinitions.HL7MsgType_EquipmentStatusMessageACK, "AE", &parseErrLocation)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}
	}

	return []byte(ackMsg), nil
}
