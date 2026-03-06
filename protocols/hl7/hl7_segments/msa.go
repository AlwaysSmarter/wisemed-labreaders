package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7MSA struct {
	FieldType          string `hl7:"MSA.0""`
	AcknowledgmentCode string `hl7:"MSA.1"`
	MessageControlID   string `hl7:"MSA.2"`
}

func (seg *HL7MSA) GetSegmentName() string {
	return "MSA"
}
func (seg *HL7MSA) CreateSegment(ackCode string, msgCtrlId string) {
	seg.FieldType = seg.GetSegmentName()
	seg.AcknowledgmentCode = ackCode
	seg.MessageControlID = msgCtrlId
}
func (seg *HL7MSA) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "MSA.0", VCheck: hl7.SpecificValue, Value: "MSA"},
		hl7.Validation{Location: "MSA.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSA.2", VCheck: hl7.HasValue},
	}
}
func (seg *HL7MSA) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "MSA.0", VCheck: hl7.SpecificValue, Value: "MSA"},
		hl7.Validation{Location: "MSA.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "MSA.2", VCheck: hl7.HasValue},
	}
}

func (seg *HL7MSA) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7MSA) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
