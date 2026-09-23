package hdhr

import (
	"os"
	"testing"
)

func TestLiveControlModel(t *testing.T) {
	if os.Getenv("OTA_LIVE") != "1" {
		t.Skip("set OTA_LIVE=1 to talk to the tuner")
	}
	c := Control{Addr: "192.168.1.252"}
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
