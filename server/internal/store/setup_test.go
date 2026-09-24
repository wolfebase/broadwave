package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestNeedsSetup(t *testing.T) {
	now := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	young := now.Add(-time.Hour)
	old := now.Add(-48 * time.Hour)

	cases := []struct {
		name               string
		complete           string
		created            time.Time
		passes, recordings int
		want               bool
	}{
		{"fresh empty", "", young, 0, 0, true},
		{"no identity yet", "", time.Time{}, 0, 0, true},
		{"finished", "1", young, 0, 0, false},
		{"day old empty", "", old, 0, 0, false},
		{"young with a pass", "", young, 1, 0, false},
		{"young with a recording", "", young, 0, 1, false},
		{"old with content still skipped", "", old, 0, 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NeedsSetup(tc.complete, tc.created, now, tc.passes, tc.recordings)
			if got != tc.want {
				t.Fatalf("NeedsSetup() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplySetupDefault(t *testing.T) {
	ctx := context.Background()
	oldDir := t.TempDir()
	old, err := Open(filepath.Join(oldDir, "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if _, err := old.Identity(ctx, "Broadwave"); err != nil {
		t.Fatal(err)
	}
	aged := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := old.db.ExecContext(ctx, `UPDATE server_identity SET created_at = ?`, aged); err != nil {
		t.Fatal(err)
	}
	needs, err := old.ApplySetupDefault(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if needs {
		t.Fatal("a catalog older than a day should not show the wizard")
	}
	settings, err := old.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["setupComplete"] != "1" {
		t.Fatalf("existing install was not stamped, got %q", settings["setupComplete"])
	}

	fresh, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if _, err := fresh.Identity(ctx, "Broadwave"); err != nil {
		t.Fatal(err)
	}
	needs, err = fresh.ApplySetupDefault(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !needs {
		t.Fatal("a new empty catalog should show the wizard")
	}
	settings, err = fresh.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["setupComplete"] == "1" {
		t.Fatal("a new catalog was marked complete before the wizard finished")
	}

	if err := fresh.UpsertDevice(ctx, hdhr.Device{DeviceID: "ABC", FriendlyName: "DUO", BaseURL: "http://127.0.0.1", TunerCount: 2}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF", Favorite: true},
	}); err != nil {
		t.Fatal(err)
	}
	needs, err = fresh.ApplySetupDefault(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !needs {
		t.Fatal("tuner lineup favorites must not dismiss the wizard")
	}
	settings, err = fresh.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings["setupComplete"] == "1" {
		t.Fatal("discovering a tuner stamped setup complete")
	}
}
