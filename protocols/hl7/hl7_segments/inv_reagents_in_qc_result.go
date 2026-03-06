package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7INV_ReagentsInQCResult struct {
	FieldType           string `hl7:"INV.0""`
	SubstanceIdentifier string `hl7:"INV.1"`
	SIControlCode       string `hl7:"INV.1.0"`
	SIControlName       string `hl7:"INV.1.1"`
	SICodingSystem      string `hl7:"INV.1.2"`
	SubstanceStatus     string `hl7:"INV.2"`
	SSSubstanceStatus   string `hl7:"INV.2.0"`
	Unused1             string `hl7:"INV.2.1"`
	SSCodingSystem      string `hl7:"INV.2.2"`
	SubstanceType       string `hl7:"INV.3"`
	STId                string `hl7:"INV.3.0"`
	Unused2             string `hl7:"INV.3.1"`
	STCodingSystem      string `hl7:"INV.3.2"`
	BottleCountNo       string `hl7:"INV.4"`
	BCNNo               string `hl7:"INV.4.0"`
	Unused3             string `hl7:"INV.4.1"`
	BCNCodingSystem     string `hl7:"INV.4.2"`
}

func (seg *HL7INV_ReagentsInQCResult) GetSegmentName() string {
	return "INV"
}
func (seg *HL7INV_ReagentsInQCResult) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}

func (seg *HL7INV_ReagentsInQCResult) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{}
}
func (seg *HL7INV_ReagentsInQCResult) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "INV.0", VCheck: hl7.SpecificValue, Value: "INV"},
		hl7.Validation{Location: "INV.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.1.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.1.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.1.2", VCheck: hl7.SpecificValue, Value: "99ROC"},
		hl7.Validation{Location: "INV.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.2.0", VCheck: hl7.SpecificValue, Value: "OK"},
		hl7.Validation{Location: "INV.2.2", VCheck: hl7.SpecificValue, Value: "HL703843"},
		hl7.Validation{Location: "INV.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.3.0", VCheck: hl7.SpecificValue, Value: "CO"},
		hl7.Validation{Location: "INV.3.2", VCheck: hl7.SpecificValue, Value: "HL703843"},
		hl7.Validation{Location: "INV.4", VCheck: hl7.HasValue},
		hl7.Validation{Location: "INV.4.0", VCheck: hl7.SpecificValue, Value: "0"},
		hl7.Validation{Location: "INV.4.2", VCheck: hl7.SpecificValue, Value: "99ROC"},
	}
}

func (seg *HL7INV_ReagentsInQCResult) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7INV_ReagentsInQCResult) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
