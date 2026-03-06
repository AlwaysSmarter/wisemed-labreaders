package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7TCD struct {
	FieldType                  string `hl7:"TCD.0""`
	UniversalServiceIdentifier string `hl7:"TCD.1""`
	USIId                      string `hl7:"TCD.1.0""`
	USIText                    string `hl7:"TCD.1.1""`
	USICodingSystem            string `hl7:"TCD.1.2""`
	AutoDilutionFactor         string `hl7:"TCD.2""`
	//ADFNum1                    string `hl7:"TCD.2.1""`
	//ADFSepSufix                string `hl7:"TCD.2.2""`
	//ADFDilutionFactor          string `hl7:"TCD.2.3""`
	Unused3      string `hl7:"TCD.3""`
	SpecimenType string `hl7:"TCD.4""`
	//SpecimenIdentifier         string `hl7:"TCD.4.0""`
	//SpecimenText               string `hl7:"TCD.4.1""`
	//SpecimenCodingSystem       string `hl7:"TCD.4.2""`
	Unused5      string `hl7:"TCD.5""`
	Unused6      string `hl7:"TCD.6""`
	Unused7      string `hl7:"TCD.7""`
	Unused8      string `hl7:"TCD.8""`
	Unused9      string `hl7:"TCD.9""`
	Unused10     string `hl7:"TCD.10""`
	SpecimenRole string `hl7:"TCD.11""`
	//SpecimenRoleId             string `hl7:"TCD.11.0""`
	//Unused11                   string `hl7:"TCD.11.1""`
	//SpecimenRoleCodingSystem   string `hl7:"TCD.11.2""`
	Unused13                   string `hl7:"TCD.13""`
	Comment                    string `hl7:"TCD.14""`
	Unused15                   string `hl7:"TCD.15""`
	Unused16                   string `hl7:"TCD.16""`
	SepcimenCollectionDatetime string `hl7:"TCD.17""`
	Unused18                   string `hl7:"TCD.18""`
	ControlExpirationDatetime  string `hl7:"TCD.19""`
	Unused20                   string `hl7:"TCD.20""`
	Unused21                   string `hl7:"TCD.21""`
	Unused22                   string `hl7:"TCD.22""`
	Unused23                   string `hl7:"TCD.23""`
	SpecimenCondition          string `hl7:"TCD.24""`
	Unused25                   string `hl7:"TCD.25""`
	Unused26                   string `hl7:"TCD.26""`
	ContainerType              string `hl7:"TCD.27""`
}

func (seg *HL7TCD) GetSegmentName() string {
	return "TCD"
}
func (seg *HL7TCD) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}

func (seg *HL7TCD) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "TCD.0", VCheck: hl7.SpecificValue, Value: "TCD"},
		hl7.Validation{Location: "TCD.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "TCD.1.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "TCD.1.2", VCheck: hl7.SpecificValue, Value: "99ROC"},
	}
}
func (seg *HL7TCD) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "TCD.0", VCheck: hl7.SpecificValue, Value: "TCD"},
		hl7.Validation{Location: "TCD.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "TCD.1.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "TCD.1.2", VCheck: hl7.SpecificValue, Value: "99ROC"},
	}
}

func (seg *HL7TCD) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7TCD) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
