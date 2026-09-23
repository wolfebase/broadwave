package hdhr

import "testing"

func TestParseStreamInfo(t *testing.T) {
	text := "3: 9.1 KMBC-HD\n4: 9.2 Me TV\n5: 62.3 Dabl\nts id=0x0669\n"
	got := ParseStreamInfo(text)
	if len(got) != 3 || got[0].GuideNumber != "9.1" || got[0].Number != 3 || got[1].Name != "Me TV" || got[2].GuideNumber != "62.3" {
		t.Fatalf("%+v", got)
	}
}

func TestFrequencyHz(t *testing.T) {
	if FrequencyHz("ch=8vsb:563000000 lock=8vsb ss=92") != 563000000 {
		t.Fatal(FrequencyHz("ch=8vsb:563000000 lock=8vsb ss=92"))
	}
	if FrequencyHz("ch=none lock=none") != 0 {
		t.Fatal("none")
	}
}
