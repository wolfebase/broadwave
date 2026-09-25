package store

import (
	"context"
	"time"
)

const (
	// SettingCheckUpdates is "0" when the daily release check is off.
	// A missing value means the check is on.
	SettingCheckUpdates   = "checkUpdates"
	SettingUpdateVersion  = "updateVersion"
	SettingUpdateNotesURL = "updateNotesURL"
	SettingUpdateMessage  = "updateMessage"
	SettingUpdateChecked  = "updateCheckedAt"
)

// UpdatesEnabled is false only when the catalog has turned the check off.
// A settings read error is returned so the caller can skip the request.
func (s *Store) UpdatesEnabled(ctx context.Context) (bool, error) {
	values, err := s.Settings(ctx)
	if err != nil {
		return false, err
	}
	return values[SettingCheckUpdates] != "0", nil
}

// SavedUpdate is the last release check. Version is empty when none is stored.
func (s *Store) SavedUpdate(ctx context.Context) (version, notesURL, message string, checked time.Time, err error) {
	values, err := s.Settings(ctx)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	return values[SettingUpdateVersion], values[SettingUpdateNotesURL], values[SettingUpdateMessage], parseSettingTime(values[SettingUpdateChecked]), nil
}

// SaveUpdate records a release check. An empty version clears the notice and keeps the time.
func (s *Store) SaveUpdate(ctx context.Context, version, notesURL, message string, checked time.Time) error {
	at := ""
	if !checked.IsZero() {
		at = checked.UTC().Format(time.RFC3339)
	}
	return s.writeSettings(ctx, map[string]string{
		SettingUpdateVersion:  version,
		SettingUpdateNotesURL: notesURL,
		SettingUpdateMessage:  message,
		SettingUpdateChecked:  at,
	})
}
