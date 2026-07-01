package shimatzugeneric

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"wisemed-labreaders/readersv3/core/module"
)

type testRuntime struct{}

func (testRuntime) ConfigPath() string             { return "" }
func (testRuntime) ReaderID() string               { return "test" }
func (testRuntime) ResolvePath(path string) string { return path }
func (testRuntime) ModuleSettings(string) map[string]interface{} {
	return map[string]interface{}{"subtype": "gc-2010"}
}
func (testRuntime) RegisterService(string, interface{}) {}
func (testRuntime) Service(string) (interface{}, bool)  { return nil, false }
func (testRuntime) Handle(string, http.Handler)         {}
func (testRuntime) Mux() *http.ServeMux                 { return http.NewServeMux() }
func (testRuntime) AddMenu(...module.MenuEntry)         {}
func (testRuntime) ConfigDir() string                   { return "" }
func (testRuntime) Logf(string, ...interface{})         {}

func TestParseShimadzuGenericGC2010(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "LCMMA 155A.TXT")
	content := `[Header]
Output Date	6/4/2026
Output Time	12:22:20 PM

[Sample Information]
Acquisition Date	5/29/2026 11:49:03 AM
Type	Unknown
Sample Name	
Sample ID	

[Compound Results (Ch1)]
# of IDs	2
ID#	Name	R.Time	Area	Height	Conc.	Curve	3rd	2nd	1st	Constant
1	alfaHCH	10.582	559890	165913	14.23912	Default	0.0000000	0.0000000	39320.57	0.0000000
2	gamaHCH	11.409	276331	107297	7.74137	Default	0.0000000	0.0000000	35695.32	0.0000000
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := parseShimadzuGeneric(path, testRuntime{})
	if err != nil {
		t.Fatal(err)
	}
	if len(data.SampleRecords) != 2 {
		t.Fatalf("expected 2 sample records, got %d", len(data.SampleRecords))
	}
	if got := data.SampleRecords[0].Record.Flags["subtype"]; got != "gc-2010" {
		t.Fatalf("expected subtype gc-2010, got %#v", got)
	}
	if got := data.SampleRecords[0].Record.SampleID; got == "" {
		t.Fatal("expected fallback sample id from file name")
	}
	if len(data.Analytes) != 2 {
		t.Fatalf("expected 2 analytes, got %d", len(data.Analytes))
	}
}
