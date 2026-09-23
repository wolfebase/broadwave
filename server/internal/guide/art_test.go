package guide

import "testing"

func TestArtLayout(t *testing.T) {
	if ArtLayout(1920, 1080, 1400) != "bleed" {
		t.Fatal("a wide 1920 picture can fill a 1400 slot")
	}
	if ArtLayout(1280, 720, 1400) != "bleed" {
		t.Fatal("1280 landscape stays inside 1.25× at 1400")
	}
	if ArtLayout(1280, 720, 1920) != "composed" {
		t.Fatal("1280 landscape must not fill a 1920 slot")
	}
	if ArtLayout(900, 500, 800) != "composed" {
		t.Fatal("under 1280 wide is composed")
	}
	if ArtLayout(1600, 2000, 800) != "composed" {
		t.Fatal("portrait is composed")
	}
	if ArtLayout(0, 0, 1400) != "composed" {
		t.Fatal("unknown size is composed")
	}
}

func TestDisplayEdge(t *testing.T) {
	if DisplayEdge(0, 800) != 0 || DisplayEdge(400, 0) != 0 {
		t.Fatal("unknown size draws at the picture's own size")
	}
	if DisplayEdge(800, 2000) != 1000 {
		t.Fatal("cap is 1.25× native")
	}
	if DisplayEdge(800, 500) != 500 {
		t.Fatal("a smaller slot is used as-is")
	}
}

func TestPickIconPrefersLandscape(t *testing.T) {
	src, w, h := PickIcon([]iconEl{
		{Src: "https://img.example/tall.png", Width: "400", Height: "800"},
		{Src: "https://img.example/wide.jpg", Width: "1920", Height: "1080"},
		{Src: "https://img.example/bigger.png", Width: "2400", Height: "3000"},
	})
	if src != "https://img.example/wide.jpg" || w != 1920 || h != 1080 {
		t.Fatalf("got %s %d×%d", src, w, h)
	}
	src, w, h = PickIcon([]iconEl{{Src: "https://img.example/only.png"}})
	if src != "https://img.example/only.png" || w != 0 || h != 0 {
		t.Fatalf("single icon %s %d×%d", src, w, h)
	}
}
