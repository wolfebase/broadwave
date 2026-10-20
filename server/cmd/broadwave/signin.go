package main

import (
	"context"
	"log/slog"
	"strings"

	"broadwave/internal/store"
)

// resetSignInEnv is the -reset-signin flag for Docker and Unraid, where
// adding a flag means editing the container's command.
const resetSignInEnv = "BROADWAVE_RESET_SIGNIN"

func envOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// resetSignIn turns device sign-in off, for a server whose admin devices are
// all lost. Paired devices keep their tokens, and a request from outside the
// house still needs one.
func resetSignIn(ctx context.Context, st *store.Store) error {
	on, err := st.DeviceAuthOn(ctx)
	if err != nil {
		return err
	}
	if on {
		if err := st.PutSettings(ctx, map[string]string{store.SettingDeviceAuth: "0"}); err != nil {
			return err
		}
		slog.Warn("reset-signin: DEVICE SIGN-IN IS NOW OFF. Anyone on the home network can use this server, settings included. Away from home still needs a paired device.")
	} else {
		slog.Warn("reset-signin: device sign-in was already off")
	}
	slog.Warn("reset-signin: pair an admin device, turn sign-in back on in Settings, Devices, then remove -reset-signin or " + resetSignInEnv + " so the next start leaves it alone")
	return nil
}
