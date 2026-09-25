package hdhr

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// DeviceHealth is the live model, firmware version, and signal lock for one device.
// The read never asks the device to install firmware.
type DeviceHealth struct {
	DeviceID        string      `json:"deviceId"`
	Model           string      `json:"model"`
	FirmwareVersion string      `json:"firmwareVersion"`
	Tuners          []TunerLock `json:"tuners"`
	Error           string      `json:"error,omitempty"`
}

// TunerLock is one tuner's signal lock. Locked is false when the device reports lock=none.
type TunerLock struct {
	Index  int  `json:"index"`
	Locked bool `json:"locked"`
}

// ReadHealth reads discover.json, /sys/version, and /tunerN/status.
// Tests point HDHR_CONTROL_PORT at the fake. A real tuner uses port 65001.
func (c *Client) ReadHealth(ctx context.Context, baseURL string) (DeviceHealth, error) {
	if err := ctx.Err(); err != nil {
		return DeviceHealth{}, err
	}
	if err := refuseInstall("/discover.json"); err != nil {
		return DeviceHealth{}, err
	}
	dev, err := c.FetchDevice(ctx, baseURL)
	if err != nil {
		return DeviceHealth{}, err
	}
	health := DeviceHealth{
		DeviceID:        dev.DeviceID,
		Model:           dev.ModelNumber,
		FirmwareVersion: dev.FirmwareVersion,
		Tuners:          []TunerLock{},
	}
	ctrl := Control{Addr: controlAddr(dev.BaseURL)}
	if err := refuseInstall("/sys/version"); err != nil {
		return DeviceHealth{}, err
	}
	if version, err := ctrl.Get("/sys/version"); err == nil {
		version = strings.TrimSpace(version)
		if version != "" {
			health.FirmwareVersion = version
		}
	}
	if health.Model == "" {
		if err := refuseInstall("/sys/model"); err != nil {
			return DeviceHealth{}, err
		}
		if model, err := ctrl.Get("/sys/model"); err == nil {
			health.Model = strings.TrimSpace(model)
		}
	}
	n := dev.TunerCount
	if n < 0 {
		n = 0
	}
	if n > 8 {
		n = 8
	}
	for i := 0; i < n; i++ {
		if err := ctx.Err(); err != nil {
			return DeviceHealth{}, err
		}
		path := fmt.Sprintf("/tuner%d/status", i)
		if err := refuseInstall(path); err != nil {
			return DeviceHealth{}, err
		}
		status, err := ctrl.Get(path)
		if err != nil {
			return DeviceHealth{}, err
		}
		health.Tuners = append(health.Tuners, TunerLock{Index: i, Locked: ParseStatus(status).Locked})
	}
	return health, nil
}

// refuseInstall blocks any read whose path would install or fetch a firmware image.
func refuseInstall(path string) error {
	low := strings.ToLower(path)
	if strings.Contains(low, "upgrade") || strings.Contains(low, "firmware") || strings.Contains(low, "sys/upgrade") {
		return fmt.Errorf("refusing %s", path)
	}
	return nil
}

func controlAddr(baseURL string) string {
	host := ""
	if u, err := url.Parse(baseURL); err == nil {
		host = u.Hostname()
	}
	if host == "" {
		host = baseURL
	}
	if p := os.Getenv("HDHR_CONTROL_PORT"); p != "" {
		return net.JoinHostPort(host, p)
	}
	return host
}
