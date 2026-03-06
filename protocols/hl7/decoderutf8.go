package hl7

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/lenaten/hl7"
	"github.com/rivo/uniseg"
	"io"
	"strings"
)

const eof = rune(0)
const endMsg = '\x0A'
const segTerm = '\x0D'

// Decoder reades hl7 messages from a stream
type DecoderUTF8 struct {
	r io.Reader
}

// NewDecoder returns a new Decoder that reades from from stream r
// Assumes one message per stream
func NewDecoderUTF8(r io.Reader) *DecoderUTF8 {
	return &DecoderUTF8{r: r}

}

const bufCap = 2048 * 200

func readBuf(reader io.Reader) ([]byte, error) {
	r := bufio.NewReader(reader)
	buf := make([]byte, 0, bufCap)
	for {
		n, err := r.Read(buf[:cap(buf)])
		buf = buf[:n]
		switch {
		case err == io.EOF:
			return buf, err
		case n < bufCap:
			return buf, nil
		case err != nil:
			return nil, err
		}
	}
}

// Split will split a set of HL7 messages
//
//	\x0b MESSAGE \x1c\x0d
func Split(buf []byte) [][]byte {
	msgSep := []byte{'\x1c', '\x0d'}
	msgs := bytes.Split(buf, msgSep)
	vmsgs := [][]byte{}
	for _, msg := range msgs {
		if len(msg) < 4 {
			continue
		}
		msg = bytes.TrimLeft(msg, "\x0b")
		msg = []byte(strings.Replace(string(msg), "\n", "\r", -1))
		vmsgs = append(vmsgs, msg)
	}
	return vmsgs
}

func (d *DecoderUTF8) ParseSep(m *hl7.Message) error {
	if len(m.Value) < 8 {
		return errors.New("Invalid message length less than 8 bytes")
	}
	if string(m.Value[:3]) != "MSH" {
		return errors.New("Invalid message: Missing MSH segment")
	}

	r := bytes.NewReader(m.Value)
	for i := 0; i < 8; i++ {
		ch, _, _ := r.ReadRune()
		if ch == eof {
			return fmt.Errorf("Invalid message: eof while parsing MSH")
		}
		switch i {
		case 3:
			m.Delimeters.Field = ch
		case 4:
			m.Delimeters.DelimeterField = string(ch)
			m.Delimeters.Component = ch
		case 5:
			m.Delimeters.DelimeterField += string(ch)
			m.Delimeters.Repetition = ch
		case 6:
			m.Delimeters.DelimeterField += string(ch)
			m.Delimeters.Escape = ch
		case 7:
			m.Delimeters.DelimeterField += string(ch)
			m.Delimeters.SubComponent = ch
		}
	}
	return nil
}

// Messages returns a new Message slice parsed from stream r
func (d *DecoderUTF8) ParseUTF8Message(m *hl7.Message) error {
	gs := uniseg.NewGraphemes(string(m.Value))
	gs.Reset()

	if err := d.ParseSep(m); err != nil {
		return err
	}
	//r := bytes.NewReader(m.Value)
	i := 0
	ii := 0
	ch := string(eof)
	for {
		err := gs.Next()
		if !err {
			ch = string(eof)
		} else {
			ch = gs.Str()
		}
		from, to := gs.Positions()
		ii += (to - from)
		switch {
		case ch == string(eof) || (ch == string(endMsg) && m.Delimeters.LFTermMsg):
			if (ii > i) && (i >= cap(m.Value) || ii >= cap(m.Value)) {
				return fmt.Errorf("unknown value. value: %s. i: %d. ii: %d", m.Value, i, ii)
			}
			if ii > i {
				v := m.Value[i:ii]
				if len(v) > 4 { // seg name + field sep
					seg := hl7.Segment{Value: v}
					seg.Parse(&m.Delimeters)
					m.Segments = append(m.Segments, seg)
				}
			}
			return nil
		case ch == string(segTerm):
			seg := hl7.Segment{Value: m.Value[i:ii]}
			seg.Parse(&m.Delimeters)
			m.Segments = append(m.Segments, seg)
			i = ii
		case ch == string(m.Delimeters.Escape):
			ii += (to - from)
			gs.Next()
		}
	}

}

// Messages returns a new Message slice parsed from stream r
func (d *DecoderUTF8) Messages() ([]*hl7.Message, error) {
	buf, err := readBuf(d.r)
	if err != nil {
		return nil, err
	}
	bufs := Split(buf)
	z := []*hl7.Message{}
	for _, buf := range bufs {
		msg := hl7.NewMessage(buf)
		if err := d.ParseUTF8Message(msg); err != nil {
			return nil, err
		}
		z = append(z, msg)
	}
	return z, nil
}
