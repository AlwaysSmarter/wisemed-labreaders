package localhttp

import (
	"testing"

	"wisemed-labreaders/readersv3/shared/debugreplay"
)

func TestParseDebugReplayScript(t *testing.T) {
	content := `2026/07/02 14:25:44.326796 demo ==> IN TCP remote=127.0.0.1 protocol=simple role=server bytes=12
TEXT:
<TRANSMIT>
abc
<==EOT
2026/07/02 14:25:45.000000 demo ==> IN TCP remote=127.0.0.1 protocol=astm role=server bytes=8
TEXT:
<ENQ><STX>1H|\^&<ETX>AA<CR><LF>
<==OUT <ACK><ACK>
2026/07/02 14:25:46.000000 demo ==> IN TCP remote=127.0.0.1 protocol=simple role=server bytes=8
TEXT:
<TRANSMIT><R>1</R>
<EOF>
`
	steps, err := parseDebugReplayScript(content)
	if err != nil {
		t.Fatalf("parseDebugReplayScript returned error: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[0].Terminator != debugreplay.TerminatorEOT {
		t.Fatalf("expected first terminator EOT, got %s", steps[0].Terminator)
	}
	if steps[1].Terminator != debugreplay.TerminatorOUT {
		t.Fatalf("expected second terminator OUT, got %s", steps[1].Terminator)
	}
	if got := string(steps[1].ExpectedOutput); got != string([]byte{0x06, 0x06}) {
		t.Fatalf("unexpected expected output bytes: %v", []byte(got))
	}
	if steps[2].Terminator != debugreplay.TerminatorEOF {
		t.Fatalf("expected third terminator EOF, got %s", steps[2].Terminator)
	}
}

func TestParseDebugReplayScriptBytewise(t *testing.T) {
	steps, err := parseDebugReplayScript("TEXT:\n<BYTEWISE>\n<STX>Q|1|^566639<ETX>\n<==OUT <ACK>\n")
	if err != nil {
		t.Fatalf("parseDebugReplayScript returned error: %v", err)
	}
	if len(steps) != 1 || !steps[0].WriteBytewise {
		t.Fatalf("bytewise replay step was not parsed: %#v", steps)
	}
}
