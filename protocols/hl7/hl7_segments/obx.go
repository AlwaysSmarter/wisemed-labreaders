package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7OBX struct {
	FieldType                     string `hl7:"OBX.0""`
	SetID                         string `hl7:"OBX.1"`
	ValueType                     string `hl7:"OBX.2"`
	ObservationIdentifier         string `hl7:"OBX.3"`
	OIId                          string `hl7:"OBX.3.0"`
	OIText                        string `hl7:"OBX.3.1"`
	OICodingSystem                string `hl7:"OBX.3.2"`
	OIAlternateId                 string `hl7:"OBX.3.3"`
	OIAlternateText               string `hl7:"OBX.3.4"`
	OIAlternateCodingSystem       string `hl7:"OBX.3.5"`
	ObservationSubId              string `hl7:"OBX.4"`
	ObservationResult             string `hl7:"OBX.5"`
	ORId                          string `hl7:"OBX.5.0"`
	Unused1                       string `hl7:"OBX.5.1"`
	ORCodingSystem                string `hl7:"OBX.5.2"`
	Unit                          string `hl7:"OBX.6"`
	UnitId                        string `hl7:"OBX.6.0"`
	Unused2                       string `hl7:"OBX.6.1"`
	UnitCodingSystem              string `hl7:"OBX.6.2"`
	Unused3                       string `hl7:"OBX.7"`
	InterpretationFlags           string `hl7:"OBX.8"`
	FlagId                        string `hl7:"OBX.8.0"`
	FlagText                      string `hl7:"OBX.8.1"`
	FlagCodingSystem              string `hl7:"OBX.8.2"`
	Unused4                       string `hl7:"OBX.9"`
	Unused5                       string `hl7:"OBX.10"`
	ResultStatus                  string `hl7:"OBX.11"`
	Unused6                       string `hl7:"OBX.12"`
	Unused7                       string `hl7:"OBX.13"`
	Unused8                       string `hl7:"OBX.14"`
	Unused9                       string `hl7:"OBX.15"`
	ResponsibleObserver           string `hl7:"OBX.16"`
	CalibObservationMetod         string `hl7:"OBX.17"`
	MeasurementUnitId             string `hl7:"OBX.18"`
	MUIEntityId                   string `hl7:"OBX.18.0"`
	MUINamespaceId                string `hl7:"OBX.18.1"`
	AnalysisDate                  string `hl7:"OBX.19"`
	Unused10                      string `hl7:"OBX.20"`
	ObservationInstanceIdentifier string `hl7:"OBX.21"`
	Unused22                      string `hl7:"OBX.22"`
	Unused23                      string `hl7:"OBX.23"`
	Unused24                      string `hl7:"OBX.24"`
	Unused25                      string `hl7:"OBX.25"`
	Unused26                      string `hl7:"OBX.26"`
	Unused27                      string `hl7:"OBX.27"`
	Unused28                      string `hl7:"OBX.28"`
	ObservationType               string `hl7:"OBX.29"`
}

func (seg *HL7OBX) GetSegmentName() string {
	return "OBX"
}
func (seg *HL7OBX) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}
func (seg *HL7OBX) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{}
}
func (seg *HL7OBX) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "OBX.0", VCheck: hl7.SpecificValue, Value: "OBX"},
		hl7.Validation{Location: "OBX.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.3", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.3.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.3.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.3.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.4", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.5", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.8", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.8.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.11", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.16", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.16.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.18", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.18.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.18.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.19", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.19.0", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBX.29", VCheck: hl7.HasValue},
	}
}

func (seg *HL7OBX) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7OBX) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {
	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
}
