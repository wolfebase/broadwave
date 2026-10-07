package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSpotsRoundTripAndKeepTheLatest(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.AddSpots(ctx, 7, [][]uint64{{1, 1 << 63, 42}, {5}}); err != nil {
		t.Fatal(err)
	}
	spots, err := st.Spots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(spots) != 2 || len(spots[0].Prints) != 3 || spots[0].Prints[1] != 1<<63 || spots[0].Prints[2] != 42 || spots[1].Prints[0] != 5 {
		t.Fatalf("%+v", spots)
	}
	if err := st.SeeSpots(ctx, []int64{spots[0].ID}); err != nil {
		t.Fatal(err)
	}
	var hits int
	if err := st.db.QueryRowContext(ctx, `SELECT hits FROM spots WHERE id = ?`, spots[0].ID).Scan(&hits); err != nil || hits != 1 {
		t.Fatalf("hits %d %v", hits, err)
	}
	many := make([][]uint64, maxSpots)
	for i := range many {
		many[i] = []uint64{uint64(i)}
	}
	if err := st.AddSpots(ctx, 8, many); err != nil {
		t.Fatal(err)
	}
	spots, err = st.Spots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(spots) != maxSpots || spots[0].Prints[0] != 0 {
		t.Fatalf("kept %d, first %+v", len(spots), spots[0])
	}
}
