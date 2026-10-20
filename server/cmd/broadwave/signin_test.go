package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestResetSignInTurnsDeviceSignInOff(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	token, err := st.EnableDeviceAuth(ctx, map[string]string{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if on, _ := st.DeviceAuthOn(ctx); !on {
		t.Fatal("sign-in did not turn on")
	}
	if err := resetSignIn(ctx, st); err != nil {
		t.Fatal(err)
	}
	if on, _ := st.DeviceAuthOn(ctx); on {
		t.Fatal("sign-in still on after the reset")
	}
	// The paired device keeps working, and a second reset changes nothing.
	if dev, revoked, err := st.ClientByToken(ctx, token); err != nil || revoked || dev.ID == "" {
		t.Fatalf("admin device lost: %v %v %+v", err, revoked, dev)
	}
	if err := resetSignIn(ctx, st); err != nil {
		t.Fatal(err)
	}
	if on, _ := st.DeviceAuthOn(ctx); on {
		t.Fatal("second reset turned sign-in on")
	}
}

func TestResetSignInEnv(t *testing.T) {
	for v, want := range map[string]bool{"1": true, "true": true, " YES ": true, "on": true, "": false, "0": false, "false": false, "no": false} {
		if got := envOn(v); got != want {
			t.Errorf("%q: %v, want %v", v, got, want)
		}
	}
}
