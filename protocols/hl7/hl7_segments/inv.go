package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7INV struct {
	FieldType             string `hl7:"INV.0""`
	TestIdentifiers       string `hl7:"INV.1"`
	TstIdentifier         string `hl7:"INV.1.0"`
	Unused1               string `hl7:"INV.1.1"`
	TstCodingSystem       string `hl7:"INV.1.2"`
	TestStatus            string `hl7:"INV.2"`
	TstStatusIdentifier   string `hl7:"INV.2.0"`
	Unused2               string `hl7:"INV.2.1"`
	TstStatusCodingSystem string `hl7:"INV.2.2"`
}

func (seg *HL7INV) GetSegmentName() string {
	return "INV"
}
func (seg *HL7INV) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}

func (seg *HL7INV) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{}
}
func (seg *HL7INV) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "INV.0", VCheck: hl7.SpecificValue, Value: "INV"},
		hl7.Validation{Location: "INV.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.1.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.1.2", VCheck: hl7.SpecificValue, Value: "99ROC"},
		hl7.Validation{Location: "INV.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.2.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.2.2", VCheck: hl7.SpecificValue, Value: "HL70383”"},
	}
}

func (seg *HL7INV) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7INV) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
