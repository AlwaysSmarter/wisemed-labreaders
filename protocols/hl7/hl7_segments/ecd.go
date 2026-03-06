package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7ECD struct {
	FieldType               string `hl7:"ECD.0""`
	RefferenceCommandNumber string `hl7:"ECD.1"`
	Instruction             string `hl7:"ECD.2"`
	CommandCode             string `hl7:"ECD.2.0"`
	Unused0                 string `hl7:"ECD.2.1"`
	NameSpace               string `hl7:"ECD.2.2"`
	Unused1                 string `hl7:"ECD.3"`
	Unused2                 string `hl7:"ECD.4"`
	CommandParameter        string `hl7:"ECD.5"`
	MaskType                string `hl7:"ECD.5.0"`
	TestCode                string `hl7:"ECD.5.1"`
	ModuleType              string `hl7:"ECD.5.2"`
	ModuleSerialNo          string `hl7:"ECD.5.3"`
	SubmoduleId             string `hl7:"ECD.5.4"`
	ReagentCode             string `hl7:"ECD.5.5"`
	ReagentLot              string `hl7:"ECD.5.6"`
	ReagentSeqNo            string `hl7:"ECD.5.7"`
}

func (seg *HL7ECD) GetSegmentName() string {
	return "ECD"
}

func (seg *HL7ECD) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}
func (seg *HL7ECD) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "ECD.0", VCheck: hl7.SpecificValue, Value: "ECD"},
		hl7.Validation{Location: "ECD.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ECD.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ECD.2.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ECD.2.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ECD.5", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ECD.5.2", VCheck: hl7.HasValue},
	}
}
func (seg *HL7ECD) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{}
}

func (seg *HL7ECD) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7ECD) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
