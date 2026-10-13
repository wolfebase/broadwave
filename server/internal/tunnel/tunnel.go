// Package tunnel runs cloudflared with a tunnel token the owner pasted into
// Settings, so the server can be opened from outside the house without a
// second container.
package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Status is what Settings shows about the tunnel.
type Status struct {
	// State is off, missing (no cloudflared), starting, connected, or error.
	State       string    `json:"state"`
	Message     string    `json:"message,omitempty"`
	Hostnames   []string  `json:"hostnames,omitempty"`
	Connections int       `json:"connections"`
	Since       time.Time `json:"since,omitzero"`
}

// Runner keeps one cloudflared running for the current token.
type Runner struct {
	// Bin is the cloudflared path. Empty means it is not installed.
	Bin string
	// start runs cloudflared. Tests replace it.
	start func(ctx context.Context, bin, token string) (io.ReadCloser, func() error, error)

	mu     sync.Mutex
	token  string
	cancel context.CancelFunc
	done   chan struct{}
	status Status
}

// New finds cloudflared next to the server, on PATH, or at
// BROADWAVE_CLOUDFLARED.
func New() *Runner {
	bin := os.Getenv("BROADWAVE_CLOUDFLARED")
	if bin == "" {
		bin, _ = exec.LookPath("cloudflared")
	}
	return &Runner{Bin: bin, status: Status{State: "off"}}
}

// Status returns a copy of the current status.
func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.status
	st.Hostnames = append([]string(nil), r.status.Hostnames...)
	if st.State == "" {
		st.State = "off"
	}
	return st
}

// Apply starts, restarts, or stops cloudflared for token. The same token
// again changes nothing.
func (r *Runner) Apply(token string) {
	r.mu.Lock()
	if token == r.token && (token == "" || r.cancel != nil || r.status.State == "missing") {
		r.mu.Unlock()
		return
	}
	cancel, done := r.cancel, r.done
	r.cancel, r.done = nil, nil
	r.token = token
	r.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if token == "" {
		r.status = Status{State: "off"}
		return
	}
	if r.Bin == "" {
		r.status = Status{State: "missing", Message: "cloudflared is not installed where the server runs. The Docker image has it."}
		return
	}
	ctx, stop := context.WithCancel(context.Background())
	r.cancel, r.done = stop, make(chan struct{})
	r.status = Status{State: "starting", Message: "Connecting to Cloudflare."}
	go r.loop(ctx, token, r.done)
}

// Stop ends cloudflared and waits for it.
func (r *Runner) Stop() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.cancel, r.done = nil, nil
	r.token = ""
	r.status = Status{State: "off"}
	r.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (r *Runner) loop(ctx context.Context, token string, done chan struct{}) {
	defer close(done)
	wait := 2 * time.Second
	for {
		began := time.Now()
		err := r.runOnce(ctx, token)
		if ctx.Err() != nil {
			return
		}
		r.mu.Lock()
		r.status.State = "error"
		r.status.Connections = 0
		if r.status.Message == "" || r.status.Message == "Connecting to Cloudflare." {
			r.status.Message = "cloudflared stopped"
			if err != nil {
				r.status.Message = fmt.Sprintf("cloudflared stopped: %v", err)
			}
		}
		r.mu.Unlock()
		if time.Since(began) > 5*time.Minute {
			wait = 2 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, time.Minute)
	}
}

func (r *Runner) runOnce(ctx context.Context, token string) error {
	start := r.start
	if start == nil {
		start = execCloudflared
	}
	out, wait, err := start(ctx, r.Bin, token)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		r.note(sc.Text())
	}
	_ = out.Close()
	return wait()
}

func execCloudflared(ctx context.Context, bin, token string) (io.ReadCloser, func() error, error) {
	// The token goes in the environment, not the arguments, so it is not in
	// the process list.
	cmd := exec.CommandContext(ctx, bin, "tunnel", "--no-autoupdate", "--loglevel", "info", "run")
	cmd.Env = append(os.Environ(), "TUNNEL_TOKEN="+token)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return nil, nil, err
	}
	waitErr := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = pw.Close()
		waitErr <- err
	}()
	return pr, func() error { return <-waitErr }, nil
}

var (
	hostnameRE = regexp.MustCompile(`\\?"hostname\\?"\s*:\s*\\?"([A-Za-z0-9.*-]+)\\?"`)
	errorRE    = regexp.MustCompile(`error="((?:[^"\\]|\\.)*)"`)
)

// note reads one cloudflared log line.
func (r *Runner) note(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case strings.Contains(line, "Registered tunnel connection"):
		r.status.Connections++
		if r.status.State != "connected" {
			r.status.Since = time.Now().UTC()
		}
		r.status.State = "connected"
		r.status.Message = ""
	case strings.Contains(line, "Unregistered tunnel connection"), strings.Contains(line, "Connection terminated"):
		if r.status.Connections > 0 {
			r.status.Connections--
		}
		if r.status.Connections == 0 && r.status.State == "connected" {
			r.status.State = "starting"
			r.status.Message = "Reconnecting to Cloudflare."
		}
	case strings.Contains(line, "Updated to new configuration"):
		var names []string
		for _, m := range hostnameRE.FindAllStringSubmatch(line, -1) {
			names = append(names, strings.ToLower(m[1]))
		}
		r.status.Hostnames = names
	case strings.Contains(line, " ERR "):
		msg := line
		if m := errorRE.FindStringSubmatch(line); m != nil {
			msg = strings.ReplaceAll(m[1], `\"`, `"`)
		} else if _, rest, ok := strings.Cut(line, " ERR "); ok {
			msg = strings.TrimSpace(rest)
		}
		if msg == "context canceled" {
			// Follows the real error on every failed connection.
			return
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		if r.status.State != "connected" {
			r.status.State = "error"
			r.status.Message = friendly(msg)
		}
		slog.Warn("tunnel: " + msg)
	}
}

func friendly(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "unauthorized") || strings.Contains(low, "invalid tunnel secret") ||
		strings.Contains(low, "control stream encountered a failure"):
		// A token for a deleted tunnel, or a wrong secret, ends the control
		// stream before the connection registers.
		return "Cloudflare did not accept the token. Copy it again from the tunnel's page."
	case strings.Contains(low, "no such host") || strings.Contains(low, "dial tcp") || strings.Contains(low, "timeout"):
		return "This server could not reach Cloudflare. Check its internet connection."
	}
	return msg
}
