package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7QAK struct {
	FieldType           string `hl7:"QAK.0""`
	QueryTag            string `hl7:"QAK.1""`
	QueryResponseStatus string `hl7:"QAK.2""`
	MessageQueryName    string `hl7:"QAK.3""`
}

func (seg *HL7QAK) GetSegmentName() string {
	return "QAK"
}
func (seg *HL7QAK) CreateSegment(responseStatus string) {
	seg.FieldType = seg.GetSegmentName()
	if responseStatus == "" {
		responseStatus = "OK"
	}
	seg.QueryResponseStatus = responseStatus

}
func (seg *HL7QAK) CopyFromQPD(qpd HL7QPD, queryResponseStatus string) {
	seg.CreateSegment(queryResponseStatus)
	seg.QueryTag = qpd.QueryTag
	seg.MessageQueryName = qpd.MessageName
}

func (seg *HL7QAK) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "QAK.0", VCheck: hl7.SpecificValue, Value: "QAK"},
		hl7.Validation{Location: "QAK.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.3.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.3.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.3.2", VCheck: hl7.HasValue},
	}
}
func (seg *HL7QAK) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "QAK.0", VCheck: hl7.SpecificValue, Value: "QAK"},
		hl7.Validation{Location: "QAK.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.3.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.3.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "QAK.3.2", VCheck: hl7.HasValue},
	}
}

func (seg *HL7QAK) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7QAK) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
