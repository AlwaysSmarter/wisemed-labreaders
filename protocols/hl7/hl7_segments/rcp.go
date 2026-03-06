package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7RCP struct {
	FieldType        string `hl7:"RCP.0""`
	QueryPriority    string `hl7:"RCP.1""`
	QueryLimitedReq  string `hl7:"RCP.2""`
	ResponseModality string `hl7:"RCP.3""`
}

func (seg *HL7RCP) GetSegmentName() string {
	return "RCP"
}
func (seg *HL7RCP) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}

func (seg *HL7RCP) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "RCP.0", VCheck: hl7.SpecificValue, Value: "RCP"},
		hl7.Validation{Location: "RCP.1", VCheck: hl7.HasValue},
	}
}
func (seg *HL7RCP) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "RCP.0", VCheck: hl7.SpecificValue, Value: "RCP"},
		hl7.Validation{Location: "RCP.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "RCP.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "RCP.3", VCheck: hl7.SpecificValue, Value: "R^HL70394"},
	}
}

func (seg *HL7RCP) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7RCP) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
