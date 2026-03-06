package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7TQ1 struct {
	FieldType      string `hl7:"TQ1.0""`
	Unused1        string `hl7:"TQ1.1""`
	Unused2        string `hl7:"TQ1.2""`
	Unused3        string `hl7:"TQ1.3""`
	Unused4        string `hl7:"TQ1.4""`
	Unused5        string `hl7:"TQ1.5""`
	Unused6        string `hl7:"TQ1.6""`
	Unused7        string `hl7:"TQ1.7""`
	Unused8        string `hl7:"TQ1.8""`
	Priority       string `hl7:"TQ1.9""`
	PriorityId     string `hl7:"TQ1.9.0""`
	Unused         string `hl7:"TQ1.9.1""`
	PriorityCoding string `hl7:"TQ1.9.2""`
}

func (seg *HL7TQ1) GetSegmentName() string {
	return "TQ1"
}
func (seg *HL7TQ1) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}

func (seg *HL7TQ1) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "TQ1.0", VCheck: hl7.SpecificValue, Value: "TQ1"},
		hl7.Validation{Location: "TQ1.1", VCheck: hl7.HasValue},
		//hl7.Validation{Location: "TQ1.9.2", VCheck: hl7.SpecificValue, Value: "HL70485”"},
	}
}
func (seg *HL7TQ1) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "TQ1.0", VCheck: hl7.SpecificValue, Value: "TQ1"},
		hl7.Validation{Location: "TQ1.1", VCheck: hl7.HasValue},
		//hl7.Validation{Location: "TQ1.9.2", VCheck: hl7.SpecificValue, Value: "HL70485”"},
	}
}

func (seg *HL7TQ1) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7TQ1) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
