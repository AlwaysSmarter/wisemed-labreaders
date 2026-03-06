package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7SPM struct {
	FieldType                  string `hl7:"SPM.0""`
	SequenceNo                 string `hl7:"SPM.1""`
	SampleInformation          string `hl7:"SPM.2""`
	AssignedSpecimenId         string `hl7:"SPM.2.0""`
	SampleSeqId                string `hl7:"SPM.2.0.0""`
	SampleType                 string `hl7:"SPM.2.0.1""`
	Unused3                    string `hl7:"SPM.3""`
	SpecimenType               string `hl7:"SPM.4""`
	SpecimenIdentifier         string `hl7:"SPM.4.0""`
	SpecimenText               string `hl7:"SPM.4.1""`
	SpecimenCodingSystem       string `hl7:"SPM.4.2""`
	Unused5                    string `hl7:"SPM.5""`
	Unused6                    string `hl7:"SPM.6""`
	Unused7                    string `hl7:"SPM.7""`
	Unused8                    string `hl7:"SPM.8""`
	Unused9                    string `hl7:"SPM.9""`
	Unused10                   string `hl7:"SPM.10""`
	SpecimenRole               string `hl7:"SPM.11""`
	SpecimenRoleId             string `hl7:"SPM.11.0""`
	Unused11                   string `hl7:"SPM.11.1""`
	SpecimenRoleCodingSystem   string `hl7:"SPM.11.2""`
	Unused12                   string `hl7:"SPM.12""`
	Unused13                   string `hl7:"SPM.13""`
	Comment                    string `hl7:"SPM.14""`
	Unused15                   string `hl7:"SPM.15""`
	Unused16                   string `hl7:"SPM.16""`
	SepcimenCollectionDatetime string `hl7:"SPM.17""`
	Unused18                   string `hl7:"SPM.18""`
	ControlExpirationDatetime  string `hl7:"SPM.19""`
	Unused20                   string `hl7:"SPM.20""`
	Unused21                   string `hl7:"SPM.21""`
	Unused22                   string `hl7:"SPM.22""`
	Unused23                   string `hl7:"SPM.23""`
	SpecimenCondition          string `hl7:"SPM.24""`
	Unused25                   string `hl7:"SPM.25""`
	Unused26                   string `hl7:"SPM.26""`
	ContainerType              string `hl7:"SPM.27""`
}

func (seg *HL7SPM) GetSegmentName() string {
	return "SPM"
}
func (seg *HL7SPM) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
	seg.SequenceNo = "1"
}

func (seg *HL7SPM) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "SPM.0", VCheck: hl7.SpecificValue, Value: "SPM"},
		hl7.Validation{Location: "SPM.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.2.0.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.2.0.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.4", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.4.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.4.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.11", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.11.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.11.2", VCheck: hl7.HasValue},
	}
}
func (seg *HL7SPM) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "SPM.0", VCheck: hl7.SpecificValue, Value: "SPM"},
		hl7.Validation{Location: "SPM.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.2.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.2.0.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.2.0.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.4", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.4.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.4.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.11", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.11.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "SPM.11.2", VCheck: hl7.HasValue},
	}
}

func (seg *HL7SPM) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7SPM) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
