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
