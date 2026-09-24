package discovery

import (
	"context"
	"testing"
)

func TestKeepLocalCloudDropsStrangers(t *testing.T) {
	raw := []byte(`[
		{"DeviceID":"10611B4C","LocalIP":"192.168.1.252","FriendlyName":"HDHomeRun CONNECT DUO"},
		{"DeviceID":"DEADBEEF","LocalIP":"10.1.1.9","FriendlyName":"Someone else"}
	]`)
	got := KeepLocalCloud(context.Background(), raw, func(_ context.Context, host string) (string, bool) {
		if host == "192.168.1.252" {
			return "10611B4C", true
		}
		return "NOTTHEIRS", true
	})
	if len(got) != 1 || got[0].ID != "10611B4C" || got[0].Addr != "192.168.1.252" {
		t.Fatalf("%+v", got)
	}
}
