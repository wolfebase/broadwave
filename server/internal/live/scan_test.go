package live

import (
	"context"
	"testing"
	"time"
)

func TestWatchPreemptsScan(t *testing.T) {
	h := &Hub{}
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		h.RunScan(context.Background(), []int{593000000}, time.Minute, func(ctx context.Context, freq int) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("scan did not start")
	}
	h.preemptScan()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scan kept the tuner after preempt")
	}
}

func TestIdleWithoutScanReleasesTheGuideScan(t *testing.T) {
	h, m := testHub(t)
	f := addTestFeed(h, m, 1, "4.1")
	started := make(chan struct{})
	go h.RunScan(context.Background(), []int{m.freq}, time.Minute, func(ctx context.Context, _ int) error {
		h.mu.Lock()
		f.probing = true
		h.mu.Unlock()
		close(started)
		<-ctx.Done()
		h.mu.Lock()
		f.probing = false
		h.mu.Unlock()
		return ctx.Err()
	})
	<-started
	if h.Idle() {
		t.Fatal("a dwelling scan must count as busy")
	}
	if !h.IdleWithoutScan(2 * time.Second) {
		t.Fatal("the scan kept its tuner")
	}
}

func TestIdleWithoutScanKeepsViewers(t *testing.T) {
	h, m := testHub(t)
	f := addTestFeed(h, m, 1, "4.1")
	addTestRendition(h, f, "copy.copy", 1, time.Now())
	if h.IdleWithoutScan(100 * time.Millisecond) {
		t.Fatal("a viewer is not a scan")
	}
}
