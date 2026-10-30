package httpapi

import (
	"context"
	"testing"
)

// A device announced once is not announced again after a restart, or when
// it comes back under a new id with the same name.
func TestAnArrivalIsRaisedOnce(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	for range 3 {
		srv := &Server{Store: st}
		srv.noteArrival(ctx, "New Android TV found: Living Room.")
	}
	(&Server{Store: st}).noteArrival(ctx, "New Apple TV found: Den.")
	events, err := st.Events(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, ev := range events {
		if ev.Kind == "home" {
			count[ev.Message]++
		}
	}
	if count["New Android TV found: Living Room."] != 1 || count["New Apple TV found: Den."] != 1 {
		t.Fatalf("arrivals %v", count)
	}
}
