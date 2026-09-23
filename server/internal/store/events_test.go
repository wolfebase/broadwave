package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEventsKeepTheNewest(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	for i := 0; i < 205; i++ {
		if err := st.AddEvent(ctx, "recording", "Saved a show"); err != nil {
			t.Fatal(err)
		}
	}
	list, err := st.Events(ctx, 40)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 40 {
		t.Fatalf("len %d", len(list))
	}
	if list[0].ID < list[len(list)-1].ID {
		t.Fatal("newest should be first")
	}
	all, err := st.Events(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 200 {
		t.Fatalf("kept %d", len(all))
	}
	if err := st.AddEvent(ctx, "guide", "Guide updated, 10 airings"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddEvent(ctx, "guide", "Guide updated, 10 airings"); err != nil {
		t.Fatal(err)
	}
	guides, err := st.Events(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ev := range guides {
		if ev.Kind == "guide" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("repeated guide lines %d", n)
	}
}
