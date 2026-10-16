package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"broadwave/internal/store"
)

// bodyCache keeps the encoded answers to the reads every screen repeats (the
// guide window, the channel list, the schedule). A key names the request and
// the data versions it read, so any write that changes the answer misses the
// cache; nothing has to clear it. The body is kept plain and gzipped with
// its ETag, so a hit costs neither JSON nor compression.
type bodyCache struct {
	mu      sync.Mutex
	entries map[string]*cachedBody
}

type cachedBody struct {
	etag  string
	plain []byte
	gz    []byte
	used  time.Time
}

// bodyCacheMax bounds the entries. A guide window per client and a few lists
// fit; a client that asks for odd windows only pushes out the oldest.
const bodyCacheMax = 64

func (c *bodyCache) get(key string) *cachedBody {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry != nil {
		entry.used = time.Now()
	}
	return entry
}

func (c *bodyCache) put(key string, entry *cachedBody) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]*cachedBody{}
	}
	if len(c.entries) >= bodyCacheMax {
		oldest := ""
		for k, e := range c.entries {
			if oldest == "" || e.used.Before(c.entries[oldest].used) {
				oldest = k
			}
		}
		delete(c.entries, oldest)
	}
	entry.used = time.Now()
	c.entries[key] = entry
}

// encodeBody marshals v once and compresses it once.
func encodeBody(v any) (*cachedBody, error) {
	body, err := marshalJSON(v)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	_, _ = gz.Write(body)
	_ = gz.Close()
	return &cachedBody{etag: `"` + hex.EncodeToString(sum[:16]) + `"`, plain: body, gz: buf.Bytes()}, nil
}

// dataKey is the part of a cache key the data versions make: only the
// counters the answer reads.
func dataKey(v store.Versions, names ...string) string {
	var b strings.Builder
	for _, name := range names {
		var n int64
		switch name {
		case "guide":
			n = v.Guide
		case "channels":
			n = v.Channels
		case "recordings":
			n = v.Recordings
		case "schedule":
			n = v.Schedule
		case "settings":
			n = v.Settings
		}
		fmt.Fprintf(&b, "%s=%d;", name, n)
	}
	return b.String()
}

// serveCached answers from the cache when key is there, or builds the answer,
// keeps it, and sends it. An empty key skips the cache: the versions could
// not be read.
func (s *Server) serveCached(w http.ResponseWriter, r *http.Request, key string, build func(ctx context.Context) (any, error)) {
	var entry *cachedBody
	if key != "" {
		entry = s.bodies.get(key)
	}
	if entry == nil {
		v, err := build(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		entry, err = encodeBody(v)
		if err != nil {
			http.Error(w, "json", http.StatusInternalServerError)
			return
		}
		if key != "" {
			s.bodies.put(key, entry)
		}
	}
	writeEntry(w, r, entry)
}

func writeEntry(w http.ResponseWriter, r *http.Request, entry *cachedBody) {
	h := w.Header()
	h.Set("ETag", entry.etag)
	h.Set("Cache-Control", "private, no-cache")
	h.Set("Vary", "Accept-Encoding")
	if r.Header.Get("If-None-Match") == entry.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", "application/json; charset=utf-8")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		h.Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(entry.gz)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(entry.plain)
}

// versionKey reads the data versions for a cache key. An error leaves the
// key empty, which skips the cache.
func (s *Server) versionKey(ctx context.Context, route string, names ...string) string {
	v, err := s.Store.DataVersions(ctx)
	if err != nil {
		return ""
	}
	return route + "|" + dataKey(v, names...)
}
