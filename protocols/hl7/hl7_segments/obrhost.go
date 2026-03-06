package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7OBRHOST struct {
	FieldType                  string `hl7:"OBR.0""`
	SetID                      string `hl7:"OBR.1"`
	PlacerOrderNumber_EntityId string `hl7:"OBR.2"`
	Unused1                    string `hl7:"OBR.3"`
	UniversalServiceIdentifier string `hl7:"OBR.4"`
	USIId                      string `hl7:"OBR.4.0"`
	USIText                    string `hl7:"OBR.4.1"`
	USICodingSystem            string `hl7:"OBR.4.2"`
	Unused5                    string `hl7:"OBR.5"`
	Unused6                    string `hl7:"OBR.6"`
	Unused7                    string `hl7:"OBR.7"`
	Unused8                    string `hl7:"OBR.8"`
	Unused9                    string `hl7:"OBR.9"`
	Unused10                   string `hl7:"OBR.10"`
	SpecimenActionCode         string `hl7:"OBR.11"`
}

func (seg *HL7OBRHOST) GetSegmentName() string {
	return "OBR"
}
func (seg *HL7OBRHOST) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}
func (seg *HL7OBRHOST) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "OBR.0", VCheck: hl7.SpecificValue, Value: "OBR"},
		hl7.Validation{Location: "OBR.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBR.4", VCheck: hl7.HasValue},
	}
}
func (seg *HL7OBRHOST) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "OBR.0", VCheck: hl7.SpecificValue, Value: "OBR"},
		hl7.Validation{Location: "OBR.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBR.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBR.4", VCheck: hl7.HasValue},
	}
}

func (seg *HL7OBRHOST) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7OBRHOST) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
