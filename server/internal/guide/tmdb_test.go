package guide

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"broadwave/internal/store"
)

func TestFillImagesUsesPoster(t *testing.T) {
	var sawKey bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") == "secret" {
			sawKey = true
		}
		if r.URL.Query().Get("query") != "Jeopardy!" {
			t.Errorf("query %s", r.URL.Query().Get("query"))
		}
		_, _ = w.Write([]byte(`{"results":[{"poster_path":"/abc.jpg"}]}`))
	}))
	defer srv.Close()
	rows := fillImages(t.Context(), srv.Client(), srv.URL, "secret", []store.Airing{
		{Title: "Jeopardy!"},
		{Title: "Jeopardy!"},
		{Title: "News", ImageURL: "http://already"},
	})
	if !sawKey || rows[0].ImageURL != "https://image.tmdb.org/t/p/w342/abc.jpg" || rows[1].ImageURL != rows[0].ImageURL {
		t.Fatalf("%+v", rows)
	}
	if rows[2].ImageURL != "http://already" {
		t.Fatal(rows[2].ImageURL)
	}
	if got := FillImages(t.Context(), "", rows); got[0].Title != "Jeopardy!" {
		t.Fatal("empty key should leave the rows alone")
	}
}
