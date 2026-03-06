package hl7_handlers

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/lenaten/hl7"
	"io"
	"reflect"
	"strings"
	"wisemed-labreaders/protocols/hl7/hl7_segments"
)

const ENQ = rune(5)
const SOH = rune(1)
const STX = rune(2)
const ETX = rune(3)
const EOT = rune(4)
const ACK = rune(6)
const NAK = rune(21)
const ETB = rune(23)

const LF = rune(10)
const VT = rune(11)
const CR = rune(13)
const FS = rune(28)
const SP = rune(32)

var debugHL7Handlers = 0

type CustomHL7Encoder struct {
	Encoder *hl7.Encoder
	Writer  io.Writer
}

func (ce *CustomHL7Encoder) NewEncoder(w io.Writer) *hl7.Encoder {
	ce.Encoder = hl7.NewEncoder(w)
	ce.Writer = w
	return ce.Encoder
}

type HL7MessageTypes struct {
	HL7MsgType_EquipmentStatusMessage          string `default:"ESU^U01^ESU_U01"`
	HL7MsgType_EquipmentStatusMessageACK       string `default:"ACK^U01^ACK"`
	HL7MsgType_InventoryUpdateMessage          string `default:"INU^U05^INU_U05"`
	HL7MsgType_InventoryUpdateMessageACK       string `default:"ACK^U05^ACK"`
	HL7MsgType_InventoryRequest                string `default:"INR^U14^INR_U14"`
	HL7MsgType_InstrumentStatusUpload          string `default:"ESU^U01^ESU_U01"`
	HL7MsgType_TestSelectionInquiry            string `default:"QBP^Q11^QBP_Q11"`
	HL7MsgType_TestResultQuery                 string `default:"QBP^Q11^QBP_Q11"`
	HL7MsgType_TestOrderQuery                  string `default:"QBP^Q11^QBP_Q11"`
	HL7MsgType_CalibrationRequest              string `default:"QBP^Q11^QBP_Q11"`
	HL7MsgType_QCRequest                       string `default:"QBP^Q11^QBP_Q11"`
	HL7MsgType_ResponseMessageFromHost         string `default:"RSP^K11^RSP_K11"`
	HL7MsgType_TestSelectionInformationReceive string `default:"OML^O33^OML_O33"`
	HL7MsgType_TestSelectionInquiryACK         string `default:"ORL^O34^ORL_O42"`

	HL7MsgType_ResponseMessageForTestSelection string `default:"OML^O33^OML_O33"`
	HL7MsgType_MeasurementResults              string `default:"OUL^R22^OUL_R22"`
	HL7MsgType_ACKToResultUpload               string `default:"ACK^R22^ACK"`
	HL7MsgType_TestMaskingRequest              string `default:"EAC^U07^EAC_U07"`
	HL7MsgType_TestMaskingRequestACK           string `default:"ACK^U07^ACK"`
	HL7MsgType_CalibrationResult               string `default:"OUL^R23^OUL_R23"`

	HL7MsgType_ACKToCalibrationResultMessage string `default:"ACK^R23^ACK"`
	HL7MsgType_EquipmentStatusRequest        string `default:"ESR^U02^ESR_U02"`
}

var HL7MessageTypesDefinitions = HL7MessageTypes{
	HL7MsgType_EquipmentStatusMessage:          "ESU^U01^ESU_U01",
	HL7MsgType_EquipmentStatusMessageACK:       "ACK^U01^ACK",
	HL7MsgType_InventoryUpdateMessage:          "INU^U05^INU_U05",
	HL7MsgType_InventoryUpdateMessageACK:       "ACK^U05^ACK",
	HL7MsgType_InventoryRequest:                "INR^U14^INR_U14",
	HL7MsgType_InstrumentStatusUpload:          "ESU^U01^ESU_U01",
	HL7MsgType_TestSelectionInquiry:            "QBP^Q11^QBP_Q11",
	HL7MsgType_TestResultQuery:                 "QBP^Q11^QBP_Q11",
	HL7MsgType_TestOrderQuery:                  "QBP^Q11^QBP_Q11",
	HL7MsgType_CalibrationRequest:              "QBP^Q11^QBP_Q11",
	HL7MsgType_QCRequest:                       "QBP^Q11^QBP_Q11",
	HL7MsgType_ResponseMessageFromHost:         "RSP^K11^RSP_K11",
	HL7MsgType_ResponseMessageForTestSelection: "OML^O33^OML_O33",
	HL7MsgType_TestSelectionInformationReceive: "OML^O33^OML_O33",
	HL7MsgType_TestSelectionInquiryACK:         "ORL^O34^ORL_O42",
	HL7MsgType_MeasurementResults:              "OUL^R22^OUL_R22",

	HL7MsgType_TestMaskingRequest:            "EAC^U07^EAC_U07",
	HL7MsgType_TestMaskingRequestACK:         "ACK^U07^ACK",
	HL7MsgType_CalibrationResult:             "OUL^R23^OUL_R23",
	HL7MsgType_ACKToResultUpload:             "ACK^R22^ACK",
	HL7MsgType_ACKToCalibrationResultMessage: "ACK^R23^ACK",
	HL7MsgType_EquipmentStatusRequest:        "ESR^U02^ESR_U02",
}

func verifySegmentValidity(msg *hl7.Message, seg hl7_segments.HL7Segment) (error, *hl7_segments.HL7ErrorWithLocation) {
	if valid, failures := msg.IsValid(seg.FromCobasValidations()); !valid {
		errMsg := fmt.Sprintf("Received message has an invalid %s segment: \n%v", seg.GetSegmentName(), failures)
		fmt.Println(errMsg)
		return errors.New(errMsg), &hl7_segments.HL7ErrorWithLocation{
			Location:  failures[0].Location,
			ErrorText: errMsg,
		}
	}
	return nil, nil
}

func getHL7Packet(msg *hl7.Message) string {
	msgVal := string(msg.Value)
	if len(msgVal) > 1 && msgVal[len(msgVal)-1:len(msgVal)] == "|" {
		msgVal = msgVal[:len(msgVal)-1]
	}
	return fmt.Sprintf("%s%s%s%s%s", string(VT), msgVal, string(CR), string(FS), string(CR))
}
func getHL7PacketFromStr(str string) string {
	if len(str) > 1 && str[len(str)-1:len(str)] == "|" {
		str = str[:len(str)-1]
	}
	return fmt.Sprintf("%s%s%s%s%s", string(VT), str, string(CR), string(FS), string(CR))
}
func encodeHL7Objects(obj ...interface{}) (*hl7.Message, error) {
	var buf bytes.Buffer
	buf.Reset()

	myEnc := CustomHL7Encoder{}
	myEnc.NewEncoder(&buf)

	msg, err := encodeToHL7Message(myEnc, obj...)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

func encodeToHL7Message(e CustomHL7Encoder, obj ...interface{}) (*hl7.Message, error) {
	msg := hl7.NewMessage([]byte{})

	b, err := marshalMany(msg, obj...)
	if err != nil {
		return nil, err
	}
	i, err := e.Writer.Write(b)
	if err != nil {
		return nil, err
	}
	if i < len(b) {
		return nil, errors.New("Failed to write all bytes")
	}

	return msg, nil

}
func marshalMany(m *hl7.Message, obj ...interface{}) ([]byte, error) {
	seg := hl7.Segment{Value: []byte("MSH" + string(m.Delimeters.Field) + m.Delimeters.DelimeterField)}
	for _, it := range obj {
		seg.Parse(&m.Delimeters)
		st := reflect.ValueOf(it).Elem()
		stt := st.Type()
		numFields := st.NumField()
		for i := 0; i < numFields; i++ {
			fld := stt.Field(i)
			offset := 0
			hl7Tag := fld.Tag.Get("hl7")
			if hl7Tag[:3] == "MSH" && i == 1 {
				continue
			}
			if hl7Tag[:3] == "MSH" && i > 0 {
				offset = -1
			}
			pos := i + offset
			r := fld.Tag.Get("hl7")
			val := st.Field(i).String()
			if r != "" {
				l := hl7.NewLocation(r)
				if hl7Tag[:3] == "MSH" && i > 0 {
					l.FieldSeq = pos
				}

				if err := m.Set(l, val); err != nil {
					return nil, err
				}
			}
		}
		//fmt.Printf("Encoded %s:\n", it.(hl7_segments.HL7Segment).GetSegmentName())
	}
	return m.Value, nil
}
func returnHL7Message(prefix string, suffix string, glue string, segs []string) string {
	return fmt.Sprintf("%s%s%s%s%s%s", prefix, glue, strings.Join(segs, string(rune(10))), glue, suffix, glue)
}

// Create the ACK message specified by the messageType paarameter in response to a packet  from the analyzer
// This message from host consists of 2/3 blocks from analyzer:
//
//		MSH
//		EQU
//	    [ERR]
//
// Analyzer will not respond to this block
func buildACKResponse(msh hl7_segments.HL7MSH, messageType string, ackCode string, errWithLoc *hl7_segments.HL7ErrorWithLocation) (string, error) {
	//create response
	mshResp := msh.CopyForResponse(messageType)
	mshResp.AcceptAcknowledgementType = "NE"
	mshResp.AcceptAcknowledgementType = "AL"
	mshResp.MessageProfileID = "ROC-02^ROCHE"

	msaResp := hl7_segments.HL7MSA{}
	msaResp.CreateSegment(ackCode, mshResp.MessageControl)

	var hl7Message *hl7.Message
	var err error

	if ackCode != "AA" {
		//create the error block too
		errResp := hl7_segments.HL7ERR{}
		errResp.CreateSegment(errWithLoc.Location, errWithLoc.ErrorId, errWithLoc.ErrorText)
		hl7Message, err = encodeHL7Objects(&mshResp, &msaResp, &errResp)
		if err != nil {
			return "", err
		}
	} else {
		hl7Message, err = encodeHL7Objects(&mshResp, &msaResp)
		if err != nil {
			return "", err
		}
	}

	return getHL7Packet(hl7Message), nil
}
