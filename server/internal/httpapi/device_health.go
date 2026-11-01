package httpapi

import (
	"net/http"
	"strings"

	"broadwave/internal/hdhr"
)

// deviceHealth is model, firmware, and signal lock. It does not install firmware.
func (s *Server) deviceHealth(w http.ResponseWriter, r *http.Request) {
	devices, err := s.Store.Devices(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	client := s.HDHR
	if client == nil {
		client = &hdhr.Client{}
	}
	out := make([]hdhr.DeviceHealth, 0, len(devices))
	for _, device := range devices {
		if !strings.HasPrefix(device.BaseURL, "http://") && !strings.HasPrefix(device.BaseURL, "https://") {
			continue
		}
		// An emulator has no tuner status or control port: reading one would
		// call it a tuner that did not answer.
		if s.Hub != nil && device.TunerCount > 0 {
			if plain, open := s.Hub.PlainStreams(r.Context(), device.BaseURL, device.DeviceID); plain {
				out = append(out, hdhr.DeviceHealth{
					DeviceID:        device.DeviceID,
					Model:           device.ModelNumber,
					FirmwareVersion: device.FirmwareVersion,
					Tuners:          []hdhr.TunerLock{},
					NoStatus:        true,
					TunerCount:      device.TunerCount,
					Streams:         open,
				})
				continue
			}
		}
		health, err := client.ReadHealth(r.Context(), device.BaseURL)
		if err != nil {
			out = append(out, hdhr.DeviceHealth{
				DeviceID:        device.DeviceID,
				Model:           device.ModelNumber,
				FirmwareVersion: device.FirmwareVersion,
				Tuners:          []hdhr.TunerLock{},
				Error:           "This tuner did not answer. Check that it is on.",
			})
			continue
		}
		health.DeviceID = device.DeviceID
		if health.Model == "" {
			health.Model = device.ModelNumber
		}
		if health.FirmwareVersion == "" {
			health.FirmwareVersion = device.FirmwareVersion
		}
		if health.Tuners == nil {
			health.Tuners = []hdhr.TunerLock{}
		}
		out = append(out, health)
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}
