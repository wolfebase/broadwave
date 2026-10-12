package hdhr

import (
	"os"
	"testing"
)

func TestLiveControlModel(t *testing.T) {
	addr := os.Getenv("OTA_TUNER")
	if os.Getenv("OTA_LIVE") != "1" || addr == "" {
		t.Skip("set OTA_LIVE=1 and OTA_TUNER=<tuner address> to talk to the tuner")
	}
	c := Control{Addr: addr}
	model, err := c.Get("/sys/model")
	if err != nil {
		t.Fatal(err)
	}
	if model == "" {
		t.Fatal("empty model")
	}
	t.Log(model)
	info, err := c.Get("/tuner0/status")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(info)
}
