package hl7_segments

import (
	"bytes"
	"errors"
	"github.com/lenaten/hl7"
)

type HL7Segment interface {
	GetSegmentName() string
	FromHostValidations() []hl7.Validation
	FromCobasValidations() []hl7.Validation
}

type HL7ErrorWithLocation struct {
	Location  string
	ErrorId   string
	ErrorText string
}

func parseHL7ByteArr(fromByteStr []byte) (*hl7.Message, error) {
	reader := bytes.NewReader(fromByteStr)
	msgs, err := NewDecoderUTF8(reader).Messages()
	//msgs, err := hl7.NewDecoder(reader).Messages()
	if err != nil {
		return nil, err
	}
	if len(msgs) <= 0 {
		return nil, errors.New("NO HL7 data")
	}

	return msgs[0], nil
}
