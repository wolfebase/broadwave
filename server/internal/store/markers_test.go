package store

import (
	"context"
	"testing"
	"time"
)

func TestMarkersKeepTheirConfidence(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	id, err := st.CreateRecording(ctx, Recording{Title: "News", Status: "complete", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ReplaceMarkers(ctx, id, []Marker{{Start: 300, End: 400, Confidence: 0.6}, {Start: 10, End: 70, Confidence: 0.95}, {Start: 500, End: 560}}); err != nil {
		t.Fatal(err)
	}
	hand, err := st.AddMarker(ctx, id, 900, 960)
	if err != nil || hand.Confidence != 1 {
		t.Fatalf("%+v %v", hand, err)
	}
	got, err := st.Markers(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{0.95, 0.6, 1, 1}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i, m := range got {
		if m.Confidence != want[i] {
			t.Fatalf("%d: %+v", i, m)
		}
	}
}
