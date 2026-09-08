package signingpad

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// WiseMED's supplied SigPlusNET uses compression=0, encryption=0 and leaves
// SaveSigInfo/SavePressureData/SaveTimeData disabled. Its SigString is uppercase
// hex of CRLF-delimited total points, stroke count, X Y pairs and stroke offsets.
// Pressure zero in Signotec's callback marks the BEGINNING of a stroke (SDK §8.13).
type signaturePoints struct {
	mu      sync.Mutex
	points  [][2]int32
	strokes []int
	invalid bool
	last    time.Time
}

func (s *signaturePoints) add(x, y, pressure int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = time.Now()
	if x < 0 || y < 0 || pressure < 0 || len(s.points) >= 100000 {
		s.invalid = true
		return
	}
	if pressure == 0 || len(s.strokes) == 0 {
		s.strokes = append(s.strokes, len(s.points))
	}
	s.points = append(s.points, [2]int32{x, y})
}
func (s *signaturePoints) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points, s.strokes, s.invalid, s.last = nil, nil, false, time.Time{}
}
func (s *signaturePoints) sigString() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.invalid {
		return "", fmt.Errorf("invalid or excessive signature points")
	}
	if len(s.points) == 0 {
		return "", fmt.Errorf("signature is empty")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d\r\n%d\r\n", len(s.points), len(s.strokes))
	for _, p := range s.points {
		fmt.Fprintf(&b, "%d %d\r\n", p[0], p[1])
	}
	for _, offset := range s.strokes {
		fmt.Fprintf(&b, "%d\r\n", offset)
	}
	return strings.ToUpper(hex.EncodeToString([]byte(b.String()))), nil
}

func legacyImageType(raw []byte) string {
	var request struct {
		ImageType string `json:"img_type"`
	}
	// The original C# falls back to PNG for every unrecognized/missing value.
	_ = json.Unmarshal(raw, &request)
	switch request.ImageType {
	case "tiff", "gif", "jpg", "bmp":
		return request.ImageType
	}
	return "png"
}

func (s *signaturePoints) lastActivity() time.Time { s.mu.Lock(); defer s.mu.Unlock(); return s.last }
