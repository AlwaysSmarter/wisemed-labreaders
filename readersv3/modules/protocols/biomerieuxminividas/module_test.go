package biomerieuxminividas

import "testing"

func TestParseMiniVidasPacketExtractsResult(t *testing.T) {
	packet := []byte("\x02\x1emtrsl|\x1epi|\x1epn|\x1esi|\x1eci000267217|\x1ertTXM|\x1ernTOXO IgM|\x1ett14:03|\x1etd05/24/11|\x1eqlNegative|\x1eqn0.12|\x1dA8\x03")
	items := parseMiniVidasPacket(packet)
	if len(items) != 1 {
		t.Fatalf("results = %d, want 1", len(items))
	}
	got := items[0]
	if got.SampleID != "267217" {
		t.Fatalf("sample id = %q, want 267217", got.SampleID)
	}
	if got.AnalyteTag != "TXM" || got.AnalyteName != "TOXO IgM" {
		t.Fatalf("unexpected analyte: %#v", got)
	}
	if got.Qualitative != "Negative" || got.Quantitative != "0.12" {
		t.Fatalf("unexpected values: %#v", got)
	}
	if got.ResultDate != "05/24/11" || got.ResultTime != "14:03" {
		t.Fatalf("unexpected result time: %#v", got)
	}
}

func TestMiniVidasRunDate(t *testing.T) {
	if got, want := miniVidasRunDate("05/24/11"), "2011-05-24"; got != want {
		t.Fatalf("run date = %q, want %q", got, want)
	}
}
