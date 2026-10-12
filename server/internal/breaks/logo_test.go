package breaks

import "testing"

func TestATiedLogoWindowKeepsTheSecondItIsOn(t *testing.T) {
	got := smooth([]bool{true, true, false, false, false}, 5)
	if !got[0] || !got[1] || got[2] {
		t.Fatalf("start %v", got)
	}
	got = smooth([]bool{false, false, true, true}, 5)
	if got[0] || got[1] || !got[2] || !got[3] {
		t.Fatalf("end %v", got)
	}
	// The same tie with the center already off stays off.
	got = smooth([]bool{false, false, true, true, true}, 5)
	if got[1] {
		t.Fatalf("break second turned on: %v", got)
	}
}

func TestTheLogoStaysOnForTheFirstSecondOfTheShow(t *testing.T) {
	const n = 300
	on := func(show func(int) bool) []bool {
		edges := make([]bitmap, n)
		for i := range edges {
			edges[i] = newBitmap()
			if !show(i) {
				continue
			}
			for p := 200; p < 216; p++ {
				edges[i].set(p)
			}
		}
		return findLogo(edges)
	}
	early := on(func(i int) bool { return i < 2 || i >= 62 })
	if early == nil || !early[0] || !early[1] || early[2] {
		t.Fatalf("start %+v", early)
	}
	late := on(func(i int) bool { return i <= 237 || i >= 298 })
	if late == nil || !late[298] || !late[299] || late[297] {
		t.Fatalf("end second 297 %v 298 %v 299 %v", late != nil && late[297], late != nil && late[298], late != nil && late[299])
	}
}
