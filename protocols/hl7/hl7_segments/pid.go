package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7PID struct {
	FieldType           string `hl7:"PID.0""`
	Unused1             string `hl7:"PID.1""`
	Unused2             string `hl7:"PID.2""`
	PatientId           string `hl7:"PID.3"`
	Unused4             string `hl7:"PID.4"`
	PatientName         string `hl7:"PID.5"`
	Unused5             string `hl7:"PID.5.0"`
	Unused6             string `hl7:"PID.5.1"`
	Unused7             string `hl7:"PID.5.2"`
	Unused8             string `hl7:"PID.5.3"`
	Unused9             string `hl7:"PID.5.4"`
	Unused10            string `hl7:"PID.5.5"`
	PatientNameTypeCode string `hl7:"PID.5.6"`
	Unused11            string `hl7:"PID.6"`
	Birthdate           string `hl7:"PID.7"`
	Sex                 string `hl7:"PID.8"`
}

func (seg *HL7PID) GetSegmentName() string {
	return "PID"
}
func (seg *HL7PID) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}
func (seg *HL7PID) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "PID.0", VCheck: hl7.SpecificValue, Value: "PID"},
		hl7.Validation{Location: "PID.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "PID.3.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "PID.5", VCheck: hl7.HasValue},
		hl7.Validation{Location: "PID.5.6", VCheck: hl7.SpecificValue, Value: "U"},
	}
}
func (seg *HL7PID) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "PID.0", VCheck: hl7.SpecificValue, Value: "PID"},
		hl7.Validation{Location: "PID.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "PID.3.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "PID.5", VCheck: hl7.HasValue},
		hl7.Validation{Location: "PID.5.6", VCheck: hl7.SpecificValue, Value: "U"},
	}
}

func (seg *HL7PID) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7PID) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
