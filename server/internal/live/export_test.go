package live

import (
	"strings"
	"testing"
)

func TestExportProbesTheWholeMultiplex(t *testing.T) {
	line := strings.Join(exportCopyArgs(3, "AC3"), " ")
	if !strings.Contains(line, "-probesize 8000000") || !strings.Contains(line, "-analyzeduration 1000000") {
		t.Fatalf("export probe: %s", line)
	}
	if strings.Contains(line, "-probesize 1000000") || strings.Contains(line, "-probesize 2000000") {
		t.Fatalf("a short probe ends before the sequence header: %s", line)
	}
	if !strings.Contains(line, "-map 0:p:3") || !strings.Contains(line, "-c copy") || !strings.Contains(line, "pipe:1") {
		t.Fatalf("export copy: %s", line)
	}
	if strings.Contains(line, "max_probe_packets") {
		t.Fatalf("a 1.0 export settles a stream on one packet: %s", line)
	}
	if three := strings.Join(exportCopyArgs(0, "AC-4"), " "); !strings.Contains(three, "-max_probe_packets 1 -i pipe:0") {
		t.Fatalf("a 3.0 export waits on its caption stream: %s", three)
	}
	whole := strings.Join(exportCopyArgs(0, ""), " ")
	if !strings.Contains(whole, "-map 0 ") || strings.Contains(whole, "0:p:") {
		t.Fatalf("the whole multiplex: %s", whole)
	}
}
