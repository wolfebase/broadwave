package discovery

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestServiceInstanceStopsOnACharacter(t *testing.T) {
	if got := serviceInstance(strings.Repeat("a", 70)); len(got) != 63 || got != strings.Repeat("a", 63) {
		t.Fatalf("ascii %q", got)
	}
	got := serviceInstance(strings.Repeat("é", 63))
	if len(got) > 63 || !utf8.ValidString(got) || utf8.RuneCountInString(got) != 31 {
		t.Fatalf("cut %q (%d bytes, %d runes)", got, len(got), utf8.RuneCountInString(got))
	}
	// A character that does not fit in the last bytes is left off.
	name := strings.Repeat("a", 61) + string(rune(0x1F309))
	if got = serviceInstance(name); got != strings.Repeat("a", 61) || !utf8.ValidString(got) {
		t.Fatalf("wide character %q", got)
	}
}
