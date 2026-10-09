package store

import (
	"context"
	"slices"
	"testing"
)

func TestUpdateVirtualKeepsTheOldLineupWhenAnInsertFails(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	created, err := st.CreateVirtual(ctx, "900", "Harbor replays", []int64{11, 12})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`CREATE TRIGGER reject_second BEFORE INSERT ON virtual_items WHEN NEW.position = 1 BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateVirtual(ctx, created.ID, "custom", "", []int64{21, 22}); err == nil {
		t.Fatal("stored a lineup whose second recording was rejected")
	}
	got, err := st.Virtual(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Recordings, []int64{11, 12}) {
		t.Fatalf("lineup %v", got.Recordings)
	}
}

func TestCreateVirtualRollsBackWhenAnItemCannotBeStored(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.db.Exec(`CREATE TRIGGER reject_second BEFORE INSERT ON virtual_items WHEN NEW.position = 1 BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateVirtual(context.Background(), "900", "Harbor replays", []int64{11, 12}); err == nil {
		t.Fatal("stored a channel whose second recording was rejected")
	}
	var channels, items int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM virtual_channels`).Scan(&channels); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM virtual_items`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if channels != 0 || items != 0 {
		t.Fatalf("left %d channel(s) and %d item(s)", channels, items)
	}
}
