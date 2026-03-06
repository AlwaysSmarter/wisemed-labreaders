package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7TCDHOST struct {
	FieldType                  string `hl7:"TCD.0""`
	UniversalServiceIdentifier string `hl7:"TCD.1""`
	USIId                      string `hl7:"TCD.1.0""`
	USIText                    string `hl7:"TCD.1.1""`
	USICodingSystem            string `hl7:"TCD.1.2""`
	AutoDilutionFactor         string `hl7:"TCD.2""`
}

func (seg *HL7TCDHOST) GetSegmentName() string {
	return "TCD"
}
func (seg *HL7TCDHOST) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}

func (seg *HL7TCDHOST) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "TCD.0", VCheck: hl7.SpecificValue, Value: "TCD"},
		hl7.Validation{Location: "TCD.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "TCD.1.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "TCD.1.2", VCheck: hl7.SpecificValue, Value: "99ROC"},
	}
}
func (seg *HL7TCDHOST) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "TCD.0", VCheck: hl7.SpecificValue, Value: "TCD"},
		hl7.Validation{Location: "TCD.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "TCD.1.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "TCD.1.2", VCheck: hl7.SpecificValue, Value: "99ROC"},
	}
}

func (seg *HL7TCDHOST) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7TCDHOST) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
