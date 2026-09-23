package disk

import (
	"strings"
	"testing"
)

func TestReserve(t *testing.T) {
	if WatermarkGB("") != 10 || WatermarkGB("0") != 0 || WatermarkGB("15") != 15 {
		t.Fatalf("watermark parse")
	}
	if !BelowReserve(9e9, 10e9) || BelowReserve(11e9, 10e9) || BelowReserve(1, 0) {
		t.Fatal("reserve comparison")
	}
	err := &LowError{Free: 4_200_000_000, Need: 10_000_000_000}
	if !strings.Contains(err.Error(), "4.2 GB") || !strings.Contains(err.Error(), "10.0 GB") {
		t.Fatal(err.Error())
	}
}

func TestStatTempDir(t *testing.T) {
	space, err := Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if space.Free == 0 || space.Total == 0 || space.Free > space.Total {
		t.Fatalf("%+v", space)
	}
}
