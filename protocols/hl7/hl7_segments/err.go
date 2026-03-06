package hl7_segments

import "github.com/lenaten/hl7"

type HL7ERR struct {
	FieldType          string `hl7:"ERR.0""`
	ErrorLocation      string `hl7:"ERR.1"`
	SegmentID          string `hl7:"ERR.2.0"`
	SegmentSequence    string `hl7:"ERR.2.1"`
	FieldNumber        string `hl7:"ERR.2.2"`
	FieldRepetition    string `hl7:"ERR.2.3"`
	ComponentNumber    string `hl7:"ERR.2.4"`
	SubComponentNumber string `hl7:"ERR.2.5"`
	ErrorCode          string `hl7:"ERR.3"`
	ErrIdentifier      string `hl7:"ERR.3.0"`
	ErrText            string `hl7:"ERR.3.1"`
	ErrCodingSystem    string `hl7:"ERR.3.2"`
	Severity           string `hl7:"ERR.4"`
	VendorDefCode      string `hl7:"ERR.5"`
	VDCId              string `hl7:"ERR.5.0"`
	VDCText            string `hl7:"ERR.5.1"`
	VDCCodingSystem    string `hl7:"ERR.5.2"`
	Unused1            string `hl7:"ERR.6"`
	Unused2            string `hl7:"ERR.7"`
	UserMessage        string `hl7:"ERR.8"`
}

func (seg *HL7ERR) GetSegmentName() string {
	return "ERR"
}
func (seg *HL7ERR) CreateSegment(location string, errorId string, errorText string) {
	seg.FieldType = seg.GetSegmentName()
	seg.FieldType = seg.GetSegmentName()
	seg.ErrorLocation = location
	seg.ErrIdentifier = errorId
	seg.ErrText = errorText
	seg.ErrCodingSystem = "HL70357"
	seg.Severity = "E"
	seg.VDCCodingSystem = "99ROC"
}
func (seg *HL7ERR) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "ERR.0", VCheck: hl7.SpecificValue, Value: "ERR"},
		hl7.Validation{Location: "ERR.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ERR.3.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ERR.3.2", VCheck: hl7.SpecificValue, Value: "HL70357"},
		hl7.Validation{Location: "ERR.4", VCheck: hl7.SpecificValue, Value: "E"},
	}
}
func (seg *HL7ERR) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "ERR.0", VCheck: hl7.SpecificValue, Value: "ERR"},
		hl7.Validation{Location: "ERR.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ERR.3.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "ERR.3.2", VCheck: hl7.SpecificValue, Value: "HL70357"},
		hl7.Validation{Location: "ERR.4", VCheck: hl7.SpecificValue, Value: "E"},
	}
}
