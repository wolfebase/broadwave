package httpapi

import (
	"context"

	"broadwave/internal/live"
)

// probeJob is one recording whose length is not known yet.
type probeJob struct {
	id   int64
	path string
}

// probeLater reads a recording's length in the background. The library list
// used to run ffprobe for each such file while the request waited: a folder
// of imported files held the first list for minutes. The stored length bumps
// the recordings version, so the apps hear recordings.changed and refetch.
func (s *Server) probeLater(id int64, path string) {
	if s.Hub == nil {
		return
	}
	tool := live.FFProbePath(s.Hub.FFmpeg)
	if tool == "" {
		return
	}
	s.probeMu.Lock()
	if s.probing == nil {
		s.probing = map[int64]bool{}
	}
	if s.probing[id] {
		s.probeMu.Unlock()
		return
	}
	s.probing[id] = true
	s.probeJobs = append(s.probeJobs, probeJob{id, path})
	start := !s.probeRunning
	s.probeRunning = true
	s.probeMu.Unlock()
	if start {
		go s.runProbes(tool)
	}
}

// runProbes works through the queue one file at a time, so a large import
// does not start a process per file at once.
func (s *Server) runProbes(tool string) {
	for {
		s.probeMu.Lock()
		if len(s.probeJobs) == 0 {
			s.probeRunning = false
			s.probeMu.Unlock()
			return
		}
		job := s.probeJobs[0]
		s.probeJobs = s.probeJobs[1:]
		s.probeMu.Unlock()
		seconds, err := probeDuration(tool, job.path)
		if err != nil || seconds <= 0 {
			seconds = -1
		}
		_ = s.Store.SetDuration(context.Background(), job.id, seconds)
		s.probeMu.Lock()
		delete(s.probing, job.id)
		s.probeMu.Unlock()
	}
}

// probeDuration is live.ProbeDuration; tests stand in for ffprobe.
var probeDuration = live.ProbeDuration
