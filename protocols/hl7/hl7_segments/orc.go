package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
	"time"
)

type HL7ORC struct {
	FieldType       string `hl7:"ORC.0""`
	OrderControl    string `hl7:"ORC.1""`
	Unused1         string `hl7:"ORC.2""`
	Unused2         string `hl7:"ORC.3""`
	Unused3         string `hl7:"ORC.4""`
	OrderStatus     string `hl7:"ORC.5""`
	Unused6         string `hl7:"ORC.6""`
	Unused7         string `hl7:"ORC.7""`
	Unused8         string `hl7:"ORC.8""`
	TransactionDate string `hl7:"ORC.9""`
}

func (seg *HL7ORC) GetSegmentName() string {
	return "ORC"
}
func (seg *HL7ORC) CreateSegment(datetime bool) {
	seg.FieldType = seg.GetSegmentName()
	if datetime {
		nowt := time.Now()
		seg.TransactionDate = nowt.Format("20060102150405")
	}
}
func (seg *HL7ORC) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "ORC.0", VCheck: hl7.SpecificValue, Value: "ORC"},
		hl7.Validation{Location: "ORC.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ORC.9", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ORC.9.0", VCheck: hl7.HasValue},
	}
}
func (seg *HL7ORC) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "ORC.0", VCheck: hl7.SpecificValue, Value: "ORC"},
		hl7.Validation{Location: "ORC.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ORC.9", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ORC.9.0", VCheck: hl7.HasValue},
	}
}

func (seg *HL7ORC) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7ORC) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
