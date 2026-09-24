package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/zeroconf/v2"
)

func TestBrowseReadsALocalService(t *testing.T) {
	server, err := zeroconf.Register("WaveguideFake", "_wgfind._tcp", "local.", 9, []string{"id=fake"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	found, err := Browse(ctx, "_wgfind._tcp")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range found {
		if item.Name == "WaveguideFake" {
			return
		}
	}
	t.Fatalf("missing local service: %+v", found)
}
