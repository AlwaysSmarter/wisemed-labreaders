package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
	"time"
)

type HL7EQU struct {
	FieldType              string `hl7:"EQU.0""`
	IdentifierForEquipment string `hl7:"EQU.1"`
	EntityIdentifier       string `hl7:"EQU.1.0"`
	NamespaceId            string `hl7:"EQU.1.1"`
	EventDatetime          string `hl7:"EQU.2"`
	InstrumentStatus       string `hl7:"EQU.3"`
	StateValue             string `hl7:"EQU.3.0"`
	StateDescription       string `hl7:"EQU.3.1"`
	CodingSystem           string `hl7:"EQU.3.2"`
	InstrumentState        string `hl7:"EQU.3.3"`
	InstrumentStateDesc    string `hl7:"EQU.3.4"`
	InstrumentCodingSystem string `hl7:"EQU.3.5"`
}

func (seg *HL7EQU) GetSegmentName() string {
	return "EQU"
}

func (seg *HL7EQU) CreateSegment() {
	nowt := time.Now()
	seg.FieldType = seg.GetSegmentName()
	seg.IdentifierForEquipment = "1"
	seg.EventDatetime = nowt.Format("20060102150405-0700")
}

func (seg *HL7EQU) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "EQU.0", VCheck: hl7.SpecificValue, Value: "EQU"},
		hl7.Validation{Location: "EQU.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "EQU.2", VCheck: hl7.HasValue},
	}
}
func (seg *HL7EQU) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "EQU.0", VCheck: hl7.SpecificValue, Value: "EQU"},
		hl7.Validation{Location: "EQU.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "EQU.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "EQU.3.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "EQU.3.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "EQU.3.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "EQU.3.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "EQU.3.4", VCheck: hl7.HasValue},
		hl7.Validation{Location: "EQU.3.5", VCheck: hl7.HasValue},
	}
}

func (seg *HL7EQU) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7EQU) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
