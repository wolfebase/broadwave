package nfo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestWriteStaysInsideTheRecordingsFolder(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	media := []byte{0x47, 0x00}
	show := filepath.Join(root, "Evening News.ts")
	if err := os.WriteFile(show, media, 0o644); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 9, 28, 18, 0, 0, 0, time.UTC)
	rec := store.Recording{
		Title: "Evening News", Subtitle: "Local headlines", Description: "The evening newscast.",
		Category: "News", Status: "complete", Path: show, StartedAt: started,
	}
	ok, err := Write(root, rec)
	if err != nil || !ok {
		t.Fatalf("write %v %v", ok, err)
	}
	body, err := os.ReadFile(Side(show))
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 || body[0] != '<' {
		t.Fatalf("%s", body)
	}
	// Refresh replaces the file.
	rec.Description = "Updated & kept"
	n, err := Refresh(root, []store.Recording{rec})
	if err != nil || n != 1 {
		t.Fatalf("refresh %d %v", n, err)
	}
	again, err := os.ReadFile(Side(show))
	if err != nil || len(again) == 0 {
		t.Fatal(err)
	}

	busy := rec
	busy.Status = "recording"
	if ok, err := Write(root, busy); err != nil || ok {
		t.Fatalf("in progress wrote %v %v", ok, err)
	}

	secret := filepath.Join(dir, "secret.ts")
	if err := os.WriteFile(secret, media, 0o644); err != nil {
		t.Fatal(err)
	}
	outside := rec
	outside.Path = secret
	if ok, err := Write(root, outside); err != nil || ok {
		t.Fatalf("outside wrote %v %v", ok, err)
	}
	if _, err := os.Stat(Side(secret)); !os.IsNotExist(err) {
		t.Fatalf("nfo left the folder: %v", err)
	}

	escape := rec
	escape.Path = root + "/../secret.ts"
	if ok, err := Write(root, escape); err != nil || ok {
		t.Fatalf("dotdot wrote %v %v", ok, err)
	}

	link := filepath.Join(root, "link.ts")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	linked := rec
	linked.Path = link
	if ok, err := Write(root, linked); err != nil || ok {
		t.Fatalf("symlink wrote %v %v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(root, "link.nfo")); !os.IsNotExist(err) {
		t.Fatalf("nfo beside the link: %v", err)
	}
	if _, err := os.Stat(Side(secret)); !os.IsNotExist(err) {
		t.Fatalf("nfo beside the target: %v", err)
	}

	shelf := filepath.Join(dir, "shelf")
	if err := os.MkdirAll(shelf, 0o755); err != nil {
		t.Fatal(err)
	}
	movie := filepath.Join(shelf, "Movie.mp4")
	if err := os.WriteFile(movie, media, 0o644); err != nil {
		t.Fatal(err)
	}
	library := rec
	library.Path = movie
	library.Category = "Movies"
	if ok, err := Write(root, library); err != nil || ok {
		t.Fatalf("library folder wrote %v %v", ok, err)
	}
	if _, err := os.Stat(Side(movie)); !os.IsNotExist(err) {
		t.Fatalf("nfo in the library folder: %v", err)
	}

	// A link in a library folder that points at a recording gets no sidecar there.
	shelfLink := filepath.Join(shelf, "News.ts")
	if err := os.Symlink(show, shelfLink); err != nil {
		t.Fatal(err)
	}
	fromShelf := rec
	fromShelf.Path = shelfLink
	if ok, err := Write(root, fromShelf); err != nil || ok {
		t.Fatalf("library link wrote %v %v", ok, err)
	}
	if _, err := os.Lstat(Side(shelfLink)); !os.IsNotExist(err) {
		t.Fatalf("nfo in the library folder: %v", err)
	}

	// A sidecar that is already a link is not written through.
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "Other.ts")
	if err := os.WriteFile(other, media, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, Side(other)); err != nil {
		t.Fatal(err)
	}
	linkedSide := rec
	linkedSide.Path = other
	if ok, err := Write(root, linkedSide); err != nil || ok {
		t.Fatalf("linked sidecar wrote %v %v", ok, err)
	}
	if kept, _ := os.ReadFile(target); string(kept) != "keep" {
		t.Fatalf("wrote through the link: %q", kept)
	}
}
