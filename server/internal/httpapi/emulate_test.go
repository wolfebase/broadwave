package httpapi

import (
	"net/http"
	"testing"
	"time"
)

func TestEmulatorHeaderAndIdleTimeouts(t *testing.T) {
	srv := emulatorServer(nil, http.NewServeMux())
	if srv.Addr != ":8478" {
		t.Fatalf("addr %s", srv.Addr)
	}
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("ReadHeaderTimeout %s", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout != 2*time.Minute {
		t.Fatalf("IdleTimeout %s", srv.IdleTimeout)
	}
	if srv.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout %s", srv.ReadTimeout)
	}
}
