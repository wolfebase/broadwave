package dvr

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"broadwave/internal/store"
	"broadwave/internal/tshealth"
)

// healthRate caps the read, so a finished recording's count never crowds the
// disk that other recordings are writing to.
var healthRate int64 = 48 << 20

// healthOpen is os.Open. A test substitutes a reader that removes the file mid-read.
var healthOpen = func(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

// measureHealth reads a finished recording once and stores the damage.
// The read is a plain loop at healthRate, after commercial indexing and keep
// rules, and it does not start ffmpeg. A missing file or a read that fails stores nothing and
// leaves the recording as it was.
func measureHealth(ctx context.Context, st *store.Store, rec store.Recording) {
	if st == nil || rec.ID == 0 || rec.Path == "" || rec.Status == "recording" {
		return
	}
	sum, err := readHealth(rec.Path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Error("recording health: " + err.Error())
		}
		return
	}
	if err := st.SetRecordingHealth(ctx, rec.ID, int64(sum.ContinuityErrors), int64(sum.TransportErrors), int64(sum.SyncLosses), int64(sum.Packets)); err != nil {
		slog.Error("recording health: " + err.Error())
	}
}

// readHealth copies the file through the counter in chunks. The path is
// checked between chunks, so a file removed while it is open is not stored
// as a partial count. On Unix an unlinked open file still reads to the end.
func readHealth(path string) (tshealth.Summary, error) {
	f, err := healthOpen(path)
	if err != nil {
		return tshealth.Summary{}, err
	}
	defer f.Close()
	var counter tshealth.Counter
	buf := make([]byte, 256<<10)
	start := time.Now()
	var read int64
	for {
		if _, err := os.Lstat(path); err != nil {
			return tshealth.Summary{}, err
		}
		n, err := f.Read(buf)
		read += int64(n)
		if ahead := time.Duration(float64(read)/float64(healthRate)*float64(time.Second)) - time.Since(start); ahead > 0 {
			time.Sleep(ahead)
		}
		if n > 0 {
			if _, werr := counter.Write(buf[:n]); werr != nil {
				return tshealth.Summary{}, werr
			}
		}
		if err == io.EOF {
			return counter.Summary(), nil
		}
		if err != nil {
			return tshealth.Summary{}, err
		}
	}
}
