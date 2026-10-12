package store

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRenameServerCountsRunes(t *testing.T) {
	st := openTestStore(t)
	ctx := t.Context()
	if _, err := st.Identity(ctx, "Broadwave"); err != nil {
		t.Fatal(err)
	}
	name := strings.Repeat("é", 63)
	if len(name) <= 63 || utf8.RuneCountInString(name) != 63 {
		t.Fatalf("fixture is %d bytes and %d runes", len(name), utf8.RuneCountInString(name))
	}
	if err := st.RenameServer(ctx, " "+name+" "); err != nil {
		t.Fatal(err)
	}
	got, err := st.Identity(ctx, "Broadwave")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name {
		t.Fatalf("name %q", got.Name)
	}
	if err := st.RenameServer(ctx, strings.Repeat("é", 64)); err == nil {
		t.Fatal("accepted 64 runes")
	}
	if err := st.RenameServer(ctx, " "); err == nil {
		t.Fatal("blank name")
	}
	got, err = st.Identity(ctx, "Broadwave")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != name {
		t.Fatalf("rejected rename stored %q", got.Name)
	}
}
