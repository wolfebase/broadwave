package realtime

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// Latency targets: how far behind real time a sync room plays. The shared
// timeline is broadcast wall time, so the same target works for every rendition.
// AVPlayer cannot seek closer to live than the playlist's hold-back (three
// target durations, 9 s once a segment runs past 2 s) behind a live edge that
// is itself 2-3 s old, so balanced sits past 12 s or Apple screens never catch it.
var latencies = map[string]time.Duration{
	"lowest":   6 * time.Second,
	"balanced": 13 * time.Second,
	"stable":   20 * time.Second,
}

// RoomState is the whole sync contract. At server time T the room plays media
// time AnchorMedia + (T - AnchorServer) * Rate. Times are Unix milliseconds;
// media time is the program date-time of the frame on screen.
type RoomState struct {
	Room         string  `json:"room"`
	ChannelID    int64   `json:"channelId"`
	Mode         string  `json:"mode"` // follow: shared playback, own controls; group: controls move everyone
	AnchorServer float64 `json:"anchorServer"`
	AnchorMedia  float64 `json:"anchorMedia"`
	Rate         float64 `json:"rate"`
	Latency      string  `json:"latency"`
	Version      int     `json:"version"`
	Members      int     `json:"members"`
}

// Target is the media time the room shows at server time now (Unix ms).
func (s RoomState) Target(now float64) float64 {
	return s.AnchorMedia + (now-s.AnchorServer)*s.Rate
}

type Command struct {
	Action    string  `json:"action"` // play, pause, seek, live, latency
	MediaTime float64 `json:"mediaTime,omitempty"`
	Latency   string  `json:"latency,omitempty"`
}

var ErrFollowRoom = errors.New("only group rooms take playback commands")

type Rooms struct {
	mu    sync.Mutex
	rooms map[string]*RoomState
	now   func() time.Time
}

func NewRooms() *Rooms {
	return &Rooms{rooms: map[string]*RoomState{}, now: time.Now}
}

func unixMS(t time.Time) float64 {
	return float64(t.UnixNano()) / 1e6
}

func liveAnchor(now time.Time, latency string) float64 {
	d, ok := latencies[latency]
	if !ok {
		d = latencies["balanced"]
	}
	return unixMS(now.Add(-d))
}

// Join adds a member, creating the room at the live target if it is new.
// Channel rooms ("channel:ID") follow live. Group rooms ("group:CODE") and
// multiview rooms ("multiview:ID") share controls, so pause hits every tile.
// earliest is the first program time the channel can play, in Unix ms. Zero
// means unknown. A fresh tune's first frame is newer than the latency target,
// and aiming past it makes the player pause until the wall clock catches up.
// The next member keeps that anchor. lowest, balanced, and stable still apply
// once the buffer is deep enough to hold them.
func (r *Rooms) Join(room string, channelID int64, earliest float64) RoomState {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		now := r.now()
		mode := "follow"
		if strings.HasPrefix(room, "group:") || strings.HasPrefix(room, "multiview:") {
			mode = "group"
		}
		media := liveAnchor(now, "balanced")
		anchorServer := unixMS(now)
		if earliest > media {
			// A fresh tune: start on the first frame, a moment later, so the
			// player has a little buffer before Settle eases it back further.
			media = earliest
			anchorServer += float64(startCushion / time.Millisecond)
		}
		st = &RoomState{
			Room: room, ChannelID: channelID, Mode: mode, Latency: "balanced", Rate: 1,
			AnchorServer: anchorServer, AnchorMedia: media, Version: 1,
		}
		r.rooms[room] = st
	}
	st.Members++
	return *st
}

// startCushion is how long a fresh tune waits on its first frame. Playing
// right at the edge left under a second buffered and stalled a few times in
// the first minute while the room eased back.
const startCushion = 1500 * time.Millisecond

// settleRate is how fast a follow room plays while it eases back to its
// latency target. Players follow a room by trimming their own rate by up to
// 3%, so a room that slows by 2.5% moves every screen with it and none of them
// pauses to make up the gap.
const settleRate = 0.975

// Easing is a room that Settle slowed, and how long until it reaches its target.
type Easing struct {
	State RoomState
	Until time.Duration
}

// Settle moves follow rooms on this channel from a first-frame anchor back to
// the latency target once earliest (Unix ms) is old enough to play there.
// eases reports whether a room has a screen that plays near the first frame
// (a browser); nil means every room does. Such a room keeps its current frame
// and plays at settleRate until it is on target; EndEase then puts it back to
// 1x. Jumping the target instead held every browser on a frozen picture for
// the whole gap. A room with one browser stays put: Chrome drops 4-10% of
// frames at any rate but 1x while sound plays, and one screen has nobody to
// line up with; a second screen starts the ease on the next segment.
// AVPlayer never plays that close to live, so a room of only Apple screens
// would sit unsynced for minutes of easing; it jumps instead (Until is 0), and
// each screen pauses once for its own gap. A fresh tune, a room already on
// its target or easing, a paused room, and a group room stay put. Each changed
// state is returned so members can be told; a second call is empty.
func (r *Rooms) Settle(channelID int64, earliest float64, eases func(room string) bool) []Easing {
	r.mu.Lock()
	defer r.mu.Unlock()
	if earliest <= 0 {
		return nil
	}
	var changed []Easing
	for _, st := range r.rooms {
		if st.ChannelID != channelID || st.Mode != "follow" || st.Rate != 1 {
			continue
		}
		ease := eases == nil || eases(st.Room)
		if ease && st.Members < 2 {
			continue
		}
		now := r.now()
		nowMS := unixMS(now)
		target := liveAnchor(now, st.Latency)
		if earliest > target {
			continue
		}
		// Already at the target, or further behind it. Never pull a room toward live.
		gap := st.Target(nowMS) - target
		if gap <= 500 {
			continue
		}
		if !ease {
			st.AnchorMedia, st.AnchorServer = target, nowMS
			st.Version++
			changed = append(changed, Easing{State: *st})
			continue
		}
		st.AnchorMedia, st.AnchorServer, st.Rate = st.Target(nowMS), nowMS, settleRate
		st.Version++
		until := time.Duration(gap / (1 - settleRate) * float64(time.Millisecond))
		changed = append(changed, Easing{State: *st, Until: until})
	}
	return changed
}

// EndEase puts a room that Settle slowed back to 1x on its current frame. It
// does nothing when anything else changed the room since (the version moved).
func (r *Rooms) EndEase(room string, version int) (RoomState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.rooms[room]
	if !ok || st.Version != version || st.Rate != settleRate {
		return RoomState{}, false
	}
	nowMS := unixMS(r.now())
	st.AnchorMedia, st.AnchorServer, st.Rate = st.Target(nowMS), nowMS, 1
	st.Version++
	return *st, true
}

// Leave drops a member; an empty room is forgotten.
func (r *Rooms) Leave(room string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return
	}
	st.Members--
	if st.Members <= 0 {
		delete(r.rooms, room)
	}
}

func (r *Rooms) State(room string) (RoomState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return RoomState{}, false
	}
	return *st, true
}

// Apply runs a playback command and returns the new state for every member.
func (r *Rooms) Apply(room string, c Command) (RoomState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		return RoomState{}, errors.New("not in that room")
	}
	now := r.now()
	nowMS := unixMS(now)
	if c.Action == "latency" {
		if _, ok := latencies[c.Latency]; !ok {
			return RoomState{}, errors.New("latency is lowest, balanced, or stable")
		}
		st.Latency = c.Latency
		if st.Mode == "follow" {
			st.AnchorServer, st.AnchorMedia, st.Rate = nowMS, liveAnchor(now, st.Latency), 1
		}
		st.Version++
		return *st, nil
	}
	if st.Mode != "group" {
		return RoomState{}, ErrFollowRoom
	}
	current := st.Target(nowMS)
	switch c.Action {
	case "pause":
		st.AnchorServer, st.AnchorMedia, st.Rate = nowMS, current, 0
	case "play":
		st.AnchorServer, st.AnchorMedia, st.Rate = nowMS, current, 1
	case "seek":
		limit := liveAnchor(now, "lowest")
		media := c.MediaTime
		if media > limit {
			media = limit
		}
		st.AnchorServer, st.AnchorMedia = nowMS, media
	case "live":
		st.AnchorServer, st.AnchorMedia, st.Rate = nowMS, liveAnchor(now, st.Latency), 1
	default:
		return RoomState{}, errors.New("unknown action")
	}
	st.Version++
	return *st, nil
}
