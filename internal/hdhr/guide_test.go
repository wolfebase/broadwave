package hdhr

import "testing"

func TestCompareGuide(t *testing.T) {
	got := []string{"14.10", "4.1", "14.2", "9.4", "14.1", "9.1", "5.1"}
	want := []string{"4.1", "5.1", "9.1", "9.4", "14.1", "14.2", "14.10"}
	for i := 1; i < len(got); i++ {
		for j := i; j > 0 && CompareGuide(got[j], got[j-1]) < 0; j-- {
			got[j], got[j-1] = got[j-1], got[j]
		}
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
}

func TestSplitGuide(t *testing.T) {
	major, minor := SplitGuide("14.10")
	if major != 14 || minor != 10 {
		t.Fatalf("14.10 -> %d.%d", major, minor)
	}
	major, minor = SplitGuide("702")
	if major != 702 || minor != 0 {
		t.Fatalf("702 -> %d.%d", major, minor)
	}
}
