package store

import (
	"context"
	"testing"
	"time"
)

func TestUpdateSetting(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	on, err := s.UpdatesEnabled(ctx)
	if err != nil || !on {
		t.Fatalf("default %v %v", on, err)
	}
	if err := s.PutSettings(ctx, map[string]string{"checkUpdates": "0"}); err != nil {
		t.Fatal(err)
	}
	on, err = s.UpdatesEnabled(ctx)
	if err != nil || on {
		t.Fatalf("off %v %v", on, err)
	}
	if err := s.PutSettings(ctx, map[string]string{"checkUpdates": "maybe"}); err == nil {
		t.Fatal("accepted a bad value")
	}
	if err := s.PutSettings(ctx, map[string]string{SettingUpdateVersion: "0.7"}); err == nil {
		t.Fatal("client wrote the release cache")
	}
	when := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if err := s.SaveUpdate(ctx, "0.7", "https://github.com/wolfebase/broadwave/releases/tag/v0.7", "Broadwave 0.7 is available", when); err != nil {
		t.Fatal(err)
	}
	ver, notes, message, checked, err := s.SavedUpdate(ctx)
	if err != nil || ver != "0.7" || notes == "" || message != "Broadwave 0.7 is available" || !checked.Equal(when) {
		t.Fatalf("%s %s %s %s %v", ver, notes, message, checked, err)
	}
}
