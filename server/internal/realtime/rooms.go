package realtime

import (
	"errors"
	"slices"
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
	"lowest": 6 * time.Second,
	// AVPlayer holds back about 13.2 s behind the wall clock on a live
	// channel (six seconds of hold-back plus the encode). At 13 s an Apple TV
	// that started behind the room could not reach it: 1.02x was ignored at
	// that edge, and two TVs sat 300 ms apart. At 16 s every screen starts
	// ahead, pauses once for exactly its gap, and trims have room to work.
	"balanced": 16 * time.Second,
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
	// LatencyMS is how far behind server time the room settles. A follow room
	// plays below 1x while it eases back to it; screens that cannot reach the
	// easing frame aim here, where the room will be when the ease ends.
	LatencyMS float64 `json:"latencyMs"`
	Version   int     `json:"version"`
	Members   int     `json:"members"`
	// People names who is in a group room, so every screen can show it.
	People []Person `json:"people,omitempty"`
}

// Person is one screen in a group room.
type Person struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Target is the media time the room shows at server time now (Unix ms).
func (s RoomState) Target(now float64) float64 {
	return s.AnchorMedia + (now-s.AnchorServer)*s.Rate
}

type Command struct {
	Action    string  `json:"action"` // play, pause, seek, live, latency, stalled
	MediaTime float64 `json:"mediaTime,omitempty"`
	Latency   string  `json:"latency,omitempty"`
}

var ErrFollowRoom = errors.New("only group rooms take playback commands")

type Rooms struct {
	mu      sync.Mutex
	rooms   map[string]*RoomState
	stepped map[string]time.Time
	now     func() time.Time
}

func NewRooms() *Rooms {
	return &Rooms{rooms: map[string]*RoomState{}, stepped: map[string]time.Time{}, now: time.Now}
}

// stallStep is how far a room steps back from live when its only screen, or a
// multiview tile, runs out of picture. A fresh room plays close to its first
// frame for a fast start; a channel whose segments arrive late or uneven
// stalls there again and again. The screen is frozen already, so the step
// costs nothing visible and the room settles where that channel holds.
const (
	stallStep  = 2 * time.Second
	stallQuiet = 3 * time.Second
)

func unixMS(t time.Time) float64 {
	return float64(t.UnixNano()) / 1e6
}

func latencyMS(latency string) float64 {
	d, ok := latencies[latency]
	if !ok {
		d = latencies["balanced"]
	}
	return float64(d / time.Millisecond)
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
	return r.JoinAt(room, channelID, earliest, "balanced")
}

// JoinAt is Join with the latency a new room starts at: the joining screen's
// own default. A room that exists keeps the latency it has, since every
// screen in it shows the same frame.
func (r *Rooms) JoinAt(room string, channelID int64, earliest float64, latency string) RoomState {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil {
		if _, ok := latencies[latency]; !ok {
			latency = "balanced"
		}
		now := r.now()
		mode := "follow"
		if strings.HasPrefix(room, "group:") || strings.HasPrefix(room, "multiview:") {
			mode = "group"
		}
		media := liveAnchor(now, latency)
		anchorServer := unixMS(now)
		if earliest > media {
			// A fresh tune: start on the first frame, a moment later, so the
			// player has a little buffer before Settle eases it back further.
			media = earliest
			cushion := startCushion
			if strings.HasPrefix(room, "multiview:") {
				cushion = tileCushion
			}
			anchorServer += float64(cushion / time.Millisecond)
		}
		st = &RoomState{
			Room: room, ChannelID: channelID, Mode: mode, Latency: latency, LatencyMS: latencyMS(latency), Rate: 1,
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

// tileCushion is startCushion for a multiview tile. A fresh tile rendition's
// segments arrive about 1.5 s apart, so after its sync hold a tile had 0.6-2.4 s
// buffered, and a late segment froze it until the room stepped back. Starting
// one step back lands where that first stall would have put it.
const tileCushion = startCushion + stallStep

// settleRate is how fast a follow room plays while it eases back to its
// latency target. Players follow a room by trimming their own rate by up to
// 3%, so a room that slows by 2.5% moves every screen with it and none of them
// pauses to make up the gap.
const settleRate = 0.975

// stepBackLocked moves a room that one screen plays, or a multiview, back by
// stallStep, never past its latency target. Anyone else in a shared room would
// pause for the step, so those rooms wait for Settle instead.
func (r *Rooms) stepBackLocked(room string, st *RoomState, now time.Time) RoomState {
	solo := st.Mode == "follow" && st.Members == 1
	if !solo && !strings.HasPrefix(room, "multiview:") || st.Rate == 0 {
		return *st
	}
	if now.Sub(r.stepped[room]) < stallQuiet {
		return *st
	}
	nowMS := unixMS(now)
	floor := liveAnchor(now, st.Latency)
	current := st.Target(nowMS)
	if current <= floor {
		return *st
	}
	st.AnchorMedia = max(current-float64(stallStep/time.Millisecond), floor)
	st.AnchorServer = nowMS
	st.Version++
	r.stepped[room] = now
	return *st
}

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

// Groups lists the group rooms by code.
func (r *Rooms) Groups() []RoomState {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []RoomState
	for name, st := range r.rooms {
		if strings.HasPrefix(name, "group:") {
			out = append(out, *st)
		}
	}
	slices.SortFunc(out, func(a, b RoomState) int { return strings.Compare(a.Room, b.Room) })
	return out
}

// OnChannel names every room that plays the channel.
func (r *Rooms) OnChannel(channelID int64) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for name, st := range r.rooms {
		if st.ChannelID == channelID {
			out = append(out, name)
		}
	}
	return out
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
		r.setLatencyLocked(st, c.Latency, now)
		return *st, nil
	}
	if c.Action == "stalled" {
		return r.stepBackLocked(room, st, now), nil
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

func (r *Rooms) setLatencyLocked(st *RoomState, latency string, now time.Time) {
	st.Latency, st.LatencyMS = latency, latencyMS(latency)
	if st.Mode == "follow" {
		st.AnchorServer, st.AnchorMedia, st.Rate = unixMS(now), liveAnchor(now, latency), 1
	}
	st.Version++
}

// Floor moves a room that plays closer to live than latency back to it. An
// Apple screen holds back about 13 s behind live and never reaches lowest, so
// a room it is in plays at balanced or further back. A multiview or group room
// keeps its place on a latency change, but one still closer to live than the
// floor moves back too: AVPlayer cannot reach it. It reports whether the room
// changed.
func (r *Rooms) Floor(room, latency string) (RoomState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.rooms[room]
	if st == nil || latencyMS(st.Latency) >= latencyMS(latency) {
		return RoomState{}, false
	}
	now := r.now()
	r.setLatencyLocked(st, latency, now)
	if floor := liveAnchor(now, latency); st.Mode != "follow" && st.Target(unixMS(now)) > floor {
		st.AnchorServer, st.AnchorMedia = unixMS(now), floor
	}
	return *st, true
}
