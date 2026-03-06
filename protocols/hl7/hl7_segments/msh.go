package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
	"time"
)

type HL7MSH struct {
	FieldType                      string `hl7:"MSH.0""`
	FieldSeparator                 string `hl7:"MSH.1"`
	EncodingChars                  string `hl7:"MSH.2"`
	SendingApp                     string `hl7:"MSH.3"`
	SendingFacility                string `hl7:"MSH.4"`
	ReceivingApp                   string `hl7:"MSH.5"`
	ReceivingFacility              string `hl7:"MSH.6"`
	MessageDatetime                string `hl7:"MSH.7"`
	Unused1                        string `hl7:"MSH.8"`
	MessageType                    string `hl7:"MSH.9"`
	MessageControl                 string `hl7:"MSH.10"`
	PrecessingId                   string `hl7:"MSH.11"`
	VersionId                      string `hl7:"MSH.12"`
	Unused2                        string `hl7:"MSH.13"`
	Unused3                        string `hl7:"MSH.14"`
	AcceptAcknowledgementType      string `hl7:"MSH.15"`
	ApplicationAcknowledgementType string `hl7:"MSH.16"`
	Unused4                        string `hl7:"MSH.17"`
	CharacterSet                   string `hl7:"MSH.18"`
	Unused5                        string `hl7:"MSH.19"`
	Unused6                        string `hl7:"MSH.20"`
	MessageProfileID               string `hl7:"MSH.21"`
	//MPIDEntityId                   string `hl7:"MSH.21.0"`
	//MPIDNamespaceId                string `hl7:"MSH.21.1"`
}

func (seg *HL7MSH) GetSegmentName() string {
	return "MSH"
}
func (seg *HL7MSH) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}

func (seg *HL7MSH) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "MSH.0", VCheck: hl7.SpecificValue, Value: "MSH"},
		hl7.Validation{Location: "MSH.1", VCheck: hl7.SpecificValue, Value: "|"},
		hl7.Validation{Location: "MSH.2", VCheck: hl7.SpecificValue, Value: "^~\\&"},
		hl7.Validation{Location: "MSH.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSH.5", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSH.7", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSH.9", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSH.10", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSH.11", VCheck: hl7.SpecificValue, Value: "P"},
		hl7.Validation{Location: "MSH.12", VCheck: hl7.SpecificValue, Value: "2.5.1"},
		//hl7.Validation{Location: "MSH.15", VCheck: hl7.HasValue}, //not always
		//hl7.Validation{Location: "MSH.16", VCheck: hl7.HasValue},//not always
		hl7.Validation{Location: "MSH.18", VCheck: hl7.SpecificValue, Value: "UNICODE UTF-8"},
	}
}
func (seg *HL7MSH) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "MSH.0", VCheck: hl7.SpecificValue, Value: "MSH"},
		hl7.Validation{Location: "MSH.1", VCheck: hl7.SpecificValue, Value: "|"},
		hl7.Validation{Location: "MSH.2", VCheck: hl7.SpecificValue, Value: "^~\\&"},
		hl7.Validation{Location: "MSH.7", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSH.9", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSH.10", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSH.11", VCheck: hl7.SpecificValue, Value: "P"},
		hl7.Validation{Location: "MSH.12", VCheck: hl7.SpecificValue, Value: "2.5.1"},
		//hl7.Validation{Location: "MSH.15", VCheck: hl7.HasValue},//not always
		//hl7.Validation{Location: "MSH.16", VCheck: hl7.HasValue},//not always
		hl7.Validation{Location: "MSH.18", VCheck: hl7.SpecificValue, Value: "UNICODE UTF-8"},
	}
}

func (seg *HL7MSH) CopyForResponse(msgType string) HL7MSH {
	nowt := time.Now()
	resp := HL7MSH{
		FieldType:       seg.FieldType,
		FieldSeparator:  seg.FieldSeparator,
		EncodingChars:   seg.EncodingChars,
		SendingApp:      seg.ReceivingApp,
		ReceivingApp:    seg.SendingApp,
		MessageDatetime: nowt.Format("20060102150405-0700"),
		MessageType:     msgType,
		MessageControl:  seg.MessageControl,
		PrecessingId:    "P",
		VersionId:       "2.5.1",
		CharacterSet:    "UNICODE UTF-8",
		//SendingFacility:                seg.FieldType,
		//ReceivingFacility:              seg.FieldType,
		//AcceptAcknowledgementType : "NE"
		//ApplicationAcknowledgementType : "AL"
	}
	return resp
}
func (seg *HL7MSH) CreateFromMessage(msg *hl7.Message, msgType string, sendingApp string, receivingApp string) {
	nowt := time.Now()
	seg.FieldType = seg.GetSegmentName()
	seg.FieldSeparator = string(msg.Delimeters.Field)
	seg.EncodingChars = msg.Delimeters.DelimeterField
	seg.MessageDatetime = nowt.Format("20060102150405-0700")
	seg.MessageType = msgType
	seg.MessageControl = nowt.Format("20060102150405.000000")
	seg.PrecessingId = "P"
	seg.VersionId = "2.5.1"
	seg.CharacterSet = "UNICODE UTF-8"
	if sendingApp != "" {
		seg.SendingApp = sendingApp
	}
	if receivingApp != "" {
		seg.ReceivingApp = receivingApp
	}
}

func (seg *HL7MSH) Unmarshall(fromByteStr []byte) error {
	tmpMsg, err := parseHL7ByteArr(fromByteStr)
	if err != nil {
		return err
	}
	err = tmpMsg.Unmarshal(seg)
	if err != nil {
		return err
	}
	return nil
}

func (seg *HL7MSH) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
