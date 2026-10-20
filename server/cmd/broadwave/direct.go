package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"broadwave/internal/direct"
	"broadwave/internal/httpapi"
	"broadwave/internal/store"
)

// secureAddr is the HTTPS listen address: BROADWAVE_SECURE_ADDR, else 8443
// beside the default 8477, else the HTTP port plus one (8478 is the tuner
// emulator's, so the default port does not use it).
func secureAddr(addr string) string {
	if v := strings.TrimSpace(os.Getenv("BROADWAVE_SECURE_ADDR")); v != "" {
		return v
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = ""
	}
	port := portOf(addr)
	if port == 8477 {
		return net.JoinHostPort(host, "8443")
	}
	return net.JoinHostPort(host, strconv.Itoa(port+1))
}

func acmeDirectory() string {
	switch v := strings.TrimSpace(os.Getenv("BROADWAVE_DIRECT_ACME")); v {
	case "":
		return direct.LetsEncrypt
	case "staging":
		return direct.LetsEncryptStaging
	default:
		return v
	}
}

// startDirect serves the handler over HTTPS and HTTP/2 under the server's
// secure name, and keeps the name and its certificate current. Off with
// BROADWAVE_DIRECT=0, and in the test harness, which must not register names.
// A port already in use leaves it off: apps must never be offered a name
// that does not answer.
func startDirect(ctx context.Context, st *store.Store, api *httpapi.Server, configDir, addr string, handler http.Handler) *http.Server {
	if os.Getenv("BROADWAVE_DIRECT") == "0" || os.Getenv("BROADWAVE_E2E") == "1" {
		return nil
	}
	id, err := st.Identity(ctx, httpapi.DefaultServerName())
	if err != nil {
		slog.Error(fmt.Sprintf("direct: %v", err))
		return nil
	}
	fixed := strings.TrimSpace(os.Getenv("BROADWAVE_DIRECT_ADDRS")) != ""
	if why := direct.Bridged(); why != "" && !fixed {
		// Each certificate counts against a weekly limit shared by every server.
		api.DirectOff = "The secure name is off: " + why + "."
		slog.Info("direct: " + api.DirectOff)
		return nil
	}
	listen := secureAddr(addr)
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		api.DirectOff = fmt.Sprintf("The secure name is off: port %d is in use. Set BROADWAVE_SECURE_ADDR to another port.", portOf(listen))
		slog.Warn(fmt.Sprintf("direct: the secure name is off: %v (set BROADWAVE_SECURE_ADDR to another port)", err))
		return nil
	}
	cfg := direct.Config{
		Dir:       filepath.Join(configDir, "direct"),
		API:       strings.TrimSpace(os.Getenv("BROADWAVE_DIRECT_API")),
		ServerID:  id.ID,
		Port:      portOf(listen),
		Directory: acmeDirectory(),
	}
	if fixed {
		list := direct.ParseAddrs(os.Getenv("BROADWAVE_DIRECT_ADDRS"))
		cfg.Addrs = func() []direct.Addr { return list }
	}
	svc, err := direct.New(cfg)
	if err != nil {
		_ = ln.Close()
		slog.Error(fmt.Sprintf("direct: %v", err))
		return nil
	}
	server := &http.Server{
		Handler:           traceMedia(handler),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		TLSConfig:         &tls.Config{GetCertificate: svc.GetCertificate, MinVersion: tls.VersionTLS12},
		ErrorLog:          log.New(quietTLS{}, "", 0),
	}
	api.Direct = svc
	go func() {
		if err := server.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			slog.Error(fmt.Sprintf("direct: %v", err))
		}
	}()
	go svc.Run(ctx)
	slog.Info(fmt.Sprintf("direct: secure name on %s", listen))
	return server
}

// quietTLS drops handshake failures: every app that checks the name before
// the certificate arrives, or with a stale address, would log one.
type quietTLS struct{}

func (quietTLS) Write(p []byte) (int, error) {
	if !bytes.Contains(p, []byte("TLS handshake error")) {
		slog.Warn(strings.TrimSpace(string(p)))
	}
	return len(p), nil
}

// traceMedia logs each media request on the secure name at Debug
// (BROADWAVE_LOG=debug): the low-latency player's blocking reloads and
// hinted parts, with how long each was held.
func traceMedia(next http.Handler) http.Handler {
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/media/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Debug(fmt.Sprintf("direct: %s %s %s %d %dms", r.Proto, r.Method, r.URL.RequestURI(), rec.status, time.Since(start).Milliseconds()))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
