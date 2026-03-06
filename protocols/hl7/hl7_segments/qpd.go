package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7QPD struct {
	FieldType                 string `hl7:"QPD.0""`
	MessageName               string `hl7:"QPD.1""`
	MessageNameId             string `hl7:"QPD.1.0""`
	MessageNameTest           string `hl7:"QPD.1.1""`
	MessageNameCodingSystem   string `hl7:"QPD.1.2""`
	QueryTag                  string `hl7:"QPD.2""`
	ContainerId               string `hl7:"QPD.3"`
	RackId                    string `hl7:"QPD.4"`
	PositionNo                string `hl7:"QPD.5"`
	Unused6                   string `hl7:"QPD.6"`
	Unused7                   string `hl7:"QPD.7"`
	Unused8                   string `hl7:"QPD.8"`
	Unused9                   string `hl7:"QPD.9"`
	SampleType                string `hl7:"QPD.10"`
	SampleTypeId              string `hl7:"QPD.10.0"`
	SampleTypeTxt             string `hl7:"QPD.10.1"`
	SampleTypeCoding          string `hl7:"QPD.10.2"`
	SampleContainerType       string `hl7:"QPD.11"`
	SampleContainerTypeId     string `hl7:"QPD.11.0"`
	SampleContainerTypeTxt    string `hl7:"QPD.11.1"`
	SampleContainerTypeCoding string `hl7:"QPD.11.2"`
	Priority                  string `hl7:"QPD.12"`
	QueryKind                 string `hl7:"QPD.13"`
}

func (seg *HL7QPD) GetSegmentName() string {
	return "QPD"
}
func (seg *HL7QPD) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}
func (seg *HL7QPD) CreateSegmentForResultOrderQuery(queryTag string, fileId string, sampleTypeId string, sampleTypeTxt string, sampleTypeCoding string, sampleContainerTypeId string, sampleContainerTypeTxt string, sampleContainerTypeCoding string) {
	seg.FieldType = seg.GetSegmentName()
	seg.MessageNameId = "REQSID"
	seg.MessageNameTest = "Query Sample Mode"
	seg.MessageNameCodingSystem = "99ROC"
	seg.QueryTag = queryTag
	seg.ContainerId = fileId
	seg.SampleTypeId = sampleTypeId
	seg.SampleTypeTxt = sampleTypeTxt
	seg.SampleTypeCoding = sampleTypeCoding
	seg.SampleContainerTypeId = sampleContainerTypeId
	seg.SampleContainerTypeTxt = sampleContainerTypeTxt
	seg.SampleContainerTypeCoding = sampleContainerTypeCoding
}

func (seg *HL7QPD) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "QPD.0", VCheck: hl7.SpecificValue, Value: "QPD"},
		hl7.Validation{Location: "QPD.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.1.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.1.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.1.2", VCheck: hl7.SpecificValue, Value: "99ROC"},
		hl7.Validation{Location: "QPD.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.3.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.10", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.12", VCheck: hl7.HasValue},
	}
}
func (seg *HL7QPD) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "QPD.0", VCheck: hl7.SpecificValue, Value: "QPD"},
		hl7.Validation{Location: "QPD.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.1.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.1.2", VCheck: hl7.SpecificValue, Value: "99ROC"},
		hl7.Validation{Location: "QPD.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.3.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.4", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.5", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.10", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.11", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QPD.12", VCheck: hl7.HasValue},
	}
}

func (seg *HL7QPD) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7QPD) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
