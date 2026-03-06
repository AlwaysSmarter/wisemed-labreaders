package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7SAC struct {
	FieldType         string `hl7:"SAC.0""`
	Unused1           string `hl7:"SAC.1""`
	Unused2           string `hl7:"SAC.2""`
	SampleInformation string `hl7:"SAC.3""`
	SampleSeqId       string `hl7:"SAC.3.0""`
	SampleType        string `hl7:"SAC.3.1""`
	Unused4           string `hl7:"SAC.4""`
	Unused5           string `hl7:"SAC.5""`
	Unused6           string `hl7:"SAC.6""`
	Unused7           string `hl7:"SAC.7""`
	Unused8           string `hl7:"SAC.8""`
	Unused9           string `hl7:"SAC.9""`
	RackId            string `hl7:"SAC.10""`
	PositionNo        string `hl7:"SAC.11""`
	Unused12          string `hl7:"SAC.12""`
	Unused13          string `hl7:"SAC.13""`
	Unused14          string `hl7:"SAC.14""`
	Unused15          string `hl7:"SAC.15""`
	Unused16          string `hl7:"SAC.16""`
	Unused17          string `hl7:"SAC.17""`
	Unused18          string `hl7:"SAC.18""`
	Unused19          string `hl7:"SAC.19""`
	Unused20          string `hl7:"SAC.20""`
	Unused21          string `hl7:"SAC.21""`
	Unused22          string `hl7:"SAC.22""`
	Unused23          string `hl7:"SAC.23""`
	Unused24          string `hl7:"SAC.24""`
	Unused25          string `hl7:"SAC.25""`
	Unused26          string `hl7:"SAC.26""`
	Unused27          string `hl7:"SAC.27""`
	Unused28          string `hl7:"SAC.28""`
	PreDilutionCode   string `hl7:"SAC.29""`
}

func (seg *HL7SAC) GetSegmentName() string {
	return "SAC"
}
func (seg *HL7SAC) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}

func (seg *HL7SAC) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "SAC.0", VCheck: hl7.SpecificValue, Value: "SAC"},
		hl7.Validation{Location: "SAC.3", VCheck: hl7.HasValue},
	}
}
func (seg *HL7SAC) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "SAC.0", VCheck: hl7.SpecificValue, Value: "SAC"},
		hl7.Validation{Location: "SAC.3", VCheck: hl7.HasValue},
	}
}

func (seg *HL7SAC) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7SAC) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
