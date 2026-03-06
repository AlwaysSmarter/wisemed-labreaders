package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7INV_ReagentsInTestResult struct {
	FieldType           string `hl7:"INV.0""`
	SubstanceIdentifier string `hl7:"INV.1"`
	SubstanceStatus     string `hl7:"INV.2"`
	SSSubstanceStatus   string `hl7:"INV.2.0"`
	SSStandbyCurrent    string `hl7:"INV.2.1"`
	ReagentType         string `hl7:"INV.3"`
	ReagentSeqNo        string `hl7:"INV.4"`
	ContainerCarrierId  string `hl7:"INV.5"`
	Position            string `hl7:"INV.6"`
	Unused1             string `hl7:"INV.7"`
	Unused2             string `hl7:"INV.8"`
	Unused3             string `hl7:"INV.9"`
	Unused4             string `hl7:"INV.10"`
	Unused5             string `hl7:"INV.11"`
	Expiry              string `hl7:"INV.12"`
	Unused6             string `hl7:"INV.13"`
	Unused7             string `hl7:"INV.14"`
	Unused8             string `hl7:"INV.15"`
	ReagentLotNo        string `hl7:"INV.16"`
}

func (seg *HL7INV_ReagentsInTestResult) GetSegmentName() string {
	return "INV"
}
func (seg *HL7INV_ReagentsInTestResult) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}

func (seg *HL7INV_ReagentsInTestResult) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{}
}
func (seg *HL7INV_ReagentsInTestResult) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "INV.0", VCheck: hl7.SpecificValue, Value: "INV"},
		hl7.Validation{Location: "INV.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.2.0", VCheck: hl7.SpecificValue, Value: "OK^^HL70383"},
		hl7.Validation{Location: "INV.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.4", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.5", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.6", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.12", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.16", VCheck: hl7.HasValue},
	}
}

func (seg *HL7INV_ReagentsInTestResult) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7INV_ReagentsInTestResult) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
