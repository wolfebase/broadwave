package live

import (
	"strings"
	"testing"
)

func TestExportProbesTheWholeMultiplex(t *testing.T) {
	line := strings.Join(exportCopyArgs(3), " ")
	if !strings.Contains(line, "-probesize 8000000") || !strings.Contains(line, "-analyzeduration 1000000") {
		t.Fatalf("export probe: %s", line)
	}
	if strings.Contains(line, "-probesize 1000000") || strings.Contains(line, "-probesize 2000000") {
		t.Fatalf("a short probe ends before the sequence header: %s", line)
	}
	if !strings.Contains(line, "-map 0:p:3") || !strings.Contains(line, "-c copy") || !strings.Contains(line, "pipe:1") {
		t.Fatalf("export copy: %s", line)
	}
	whole := strings.Join(exportCopyArgs(0), " ")
	if !strings.Contains(whole, "-map 0 ") || strings.Contains(whole, "0:p:") {
		t.Fatalf("the whole multiplex: %s", whole)
	}
}
