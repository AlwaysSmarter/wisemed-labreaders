package hl7_segments

import (
	"fmt"
	"github.com/lenaten/hl7"
)

type HL7OBR struct {
	FieldType                            string `hl7:"OBR.0""`
	SetID                                string `hl7:"OBR.1"`
	PlacerOrderNumber_EntityId           string `hl7:"OBR.2"`
	Unused1                              string `hl7:"OBR.3"`
	UniversalServiceIdentifier           string `hl7:"OBR.4"`
	USIId                                string `hl7:"OBR.4.0"`
	USIText                              string `hl7:"OBR.4.1"`
	USICodingSystem                      string `hl7:"OBR.4.2"`
	Unused7                              string `hl7:"OBR.5"`
	Unused2                              string `hl7:"OBR.6"`
	Unused3                              string `hl7:"OBR.7"`
	Unused4                              string `hl7:"OBR.8"`
	Unused5                              string `hl7:"OBR.9"`
	Unused6                              string `hl7:"OBR.10"`
	SpecimenActionCode                   string `hl7:"OBR.11"`
	Unused8                              string `hl7:"OBR.12"`
	Unused9                              string `hl7:"OBR.13"`
	Unused10                             string `hl7:"OBR.14"`
	Unused11                             string `hl7:"OBR.15"`
	Unused12                             string `hl7:"OBR.16"`
	Unused13                             string `hl7:"OBR.17"`
	Unused14                             string `hl7:"OBR.18"`
	Unused15                             string `hl7:"OBR.19"`
	Unused20                             string `hl7:"OBR.20"`
	Unused21                             string `hl7:"OBR.21"`
	Unused22                             string `hl7:"OBR.22"`
	Unused23                             string `hl7:"OBR.23"`
	Unused24                             string `hl7:"OBR.24"`
	Unused25                             string `hl7:"OBR.25"`
	Unused26                             string `hl7:"OBR.26"`
	Unused27                             string `hl7:"OBR.27"`
	Unused28                             string `hl7:"OBR.28"`
	Unused29                             string `hl7:"OBR.29"`
	Unused30                             string `hl7:"OBR.30"`
	Unused31                             string `hl7:"OBR.31"`
	Unused32                             string `hl7:"OBR.32"`
	Unused33                             string `hl7:"OBR.33"`
	Unused34                             string `hl7:"OBR.34"`
	Unused35                             string `hl7:"OBR.35"`
	Unused36                             string `hl7:"OBR.36"`
	Unused37                             string `hl7:"OBR.37"`
	Unused38                             string `hl7:"OBR.38"`
	Unused39                             string `hl7:"OBR.39"`
	Unused40                             string `hl7:"OBR.40"`
	Unused41                             string `hl7:"OBR.41"`
	Unused42                             string `hl7:"OBR.42"`
	Unused43                             string `hl7:"OBR.43"`
	Unused44                             string `hl7:"OBR.44"`
	Unused45                             string `hl7:"OBR.45"`
	PlacerSupplementalServiceInformation string `hl7:"OBR.46"`
	SICalibrationMethod                  string `hl7:"OBR.46.0"`
	Unused46                             string `hl7:"OBR.46.1"`
	SICodingSystem                       string `hl7:"OBR.46.2"`
}

func (seg *HL7OBR) GetSegmentName() string {
	return "OBR"
}
func (seg *HL7OBR) CreateSegment() {
	seg.FieldType = seg.GetSegmentName()
}
func (seg *HL7OBR) FromHostValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "OBR.0", VCheck: hl7.SpecificValue, Value: "OBR"},
		hl7.Validation{Location: "OBR.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBR.4", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBR.11", VCheck: hl7.HasValue},
	}
}
func (seg *HL7OBR) FromCobasValidations() []hl7.Validation {
	return []hl7.Validation{
		hl7.Validation{Location: "OBR.0", VCheck: hl7.SpecificValue, Value: "OBR"},
		hl7.Validation{Location: "OBR.1", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBR.2", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBR.4", VCheck: hl7.HasValue},
		hl7.Validation{Location: "OBR.11", VCheck: hl7.HasValue},
	}
}

func (seg *HL7OBR) Unmarshall(fromByteStr []byte) error {
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

func (seg *HL7OBR) UnmarshallFromSeg(mshSegment hl7.Segment, fromSegment hl7.Segment) error {

	return seg.Unmarshall([]byte(fmt.Sprintf("%s%s", mshSegment.Value, fromSegment.Value)))
	//
	//msgBUffer := fmt.Sprintf("%s%s%s%s%s", string(rune(11)), mshSegment.Value, fromSegment.Value, string(rune(28)), string(rune(13)))
	//hl7Block := protocols.GetPackage(&msgBUffer, string(rune(11)), fmt.Sprintf("%s%s", string(rune(28)), string(rune(13))), false, false)
	//
	//return seg.Unmarshall([]byte(hl7Block))
}
