package live

import (
	"context"
	"slices"
	"time"
)

// fileReader is a scan reading a recording's file. pruneDir is set when the
// recording was removed and the scan had not let go yet. Under playMu.
type fileReader struct {
	cancel   context.CancelFunc
	done     chan struct{}
	pruneDir string
}

// readerWait is how long removing a recording waits for a stopped scan.
var readerWait = 5 * time.Second

// ReadingRecording marks work that reads a recording's file. The context ends
// when the recording is removed, and release says the work has let go of the
// file. On a FUSE share an open file stays on disk and keeps its folder from
// emptying, so a scan of a deleted recording must not run on.
func (h *Hub) ReadingRecording(ctx context.Context, id int64) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	reader := &fileReader{cancel: cancel, done: make(chan struct{})}
	h.playMu.Lock()
	if h.fileReaders == nil {
		h.fileReaders = map[int64][]*fileReader{}
	}
	h.fileReaders[id] = append(h.fileReaders[id], reader)
	h.playMu.Unlock()
	return ctx, func() {
		cancel()
		h.playMu.Lock()
		h.fileReaders[id] = slices.DeleteFunc(h.fileReaders[id], func(r *fileReader) bool { return r == reader })
		if len(h.fileReaders[id]) == 0 {
			delete(h.fileReaders, id)
		}
		dir := reader.pruneDir
		h.playMu.Unlock()
		close(reader.done)
		if dir != "" {
			h.pruneEmpty(dir)
		}
	}
}

// stopRecordingReaders stops the recording's scans and waits briefly for them
// to let go. One that takes longer prunes dir when it does.
func (h *Hub) stopRecordingReaders(id int64, dir string) {
	h.playMu.Lock()
	readers := slices.Clone(h.fileReaders[id])
	for _, reader := range readers {
		reader.pruneDir = dir
	}
	h.playMu.Unlock()
	deadline := time.After(readerWait)
	for _, reader := range readers {
		reader.cancel()
	}
	for _, reader := range readers {
		select {
		case <-reader.done:
		case <-deadline:
			return
		}
	}
}
