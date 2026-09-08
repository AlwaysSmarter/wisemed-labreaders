package signingpad

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSigStringMatchesOriginalTopazDLL(t *testing.T) {
	raw, err := os.ReadFile("testdata/topaz-sigstring.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Points [][3]int32 `json:"points"`
		Sigenc string     `json:"sigenc"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	var points signaturePoints
	for _, point := range fixture.Points {
		points.add(point[0], point[1], point[2])
	}
	actual, err := points.sigString()
	if err != nil || actual != fixture.Sigenc {
		t.Fatalf("%s, %v", actual, err)
	}
	points.reset()
	if _, err := points.sigString(); err == nil {
		t.Fatal("empty signature accepted")
	}
	if !points.lastActivity().IsZero() {
		t.Fatal("retry retained timer")
	}
	points.add(-1, 2, 0)
	if _, err := points.sigString(); err == nil {
		t.Fatal("invalid points accepted")
	}
}

func TestLegacyImageTypes(t *testing.T) {
	for _, imageType := range []string{"tiff", "gif", "jpg", "bmp", "png", "jpeg", "PNG", ""} {
		raw, _ := json.Marshal(map[string]string{"img_type": imageType})
		expected := imageType
		if imageType == "jpeg" || imageType == "PNG" || imageType == "" {
			expected = "png"
		}
		if actual := legacyImageType(raw); actual != expected {
			t.Fatalf("%s -> %s", imageType, actual)
		}
	}
}
