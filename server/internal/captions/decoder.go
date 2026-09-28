package captions

import "strings"

const (
	rows = 15
	cols = 32
)

type mode int

const (
	popOn mode = iota
	rollUp
	paintOn
)

// Cue is caption text on screen from Start until End, in 90 kHz PTS.
type Cue struct {
	Start, End int64
	Text       string
	// Top is set when the caption sits in the upper half of the screen.
	Top bool
}

type screen [rows][cols]rune

func (s *screen) clear() { *s = screen{} }

// Decoder is a CEA-608 decoder for caption channel 1 (CC1). Feed it each
// picture's field-1 pairs in display order; Take returns the cues that ended.
type Decoder struct {
	shown, hidden screen
	mode          mode
	depth         int // roll-up rows
	row, col      int
	channel       int
	text          bool // text service (TR/RTD): not captions
	last          [2]byte

	open bool
	cue  Cue
	done []Cue
	// typedAt is when text being typed was last shown; typed is false until
	// the first time on a line, which shows at once.
	typedAt int64
	typed   bool
	waiting bool
}

// shownEvery is the least time between two showings of paint-on and roll-up
// text while it is typed (5 a second). Showing every character would make a
// cue for each one; showing less often puts the words behind the picture.
const shownEvery = 18000

func NewDecoder() *Decoder {
	return &Decoder{row: rows - 1, channel: 1, depth: 3}
}

// Feed decodes one picture's caption pairs shown at pts.
func (d *Decoder) Feed(pts int64, pairs []byte) {
	d.due(pts)
	for i := 0; i+1 < len(pairs); i += 2 {
		d.pair(pts, pairs[i], pairs[i+1])
	}
}

// Take returns the cues that ended since the last call.
func (d *Decoder) Take() []Cue {
	out := d.done
	d.done = nil
	return out
}

// Showing is the cue on screen now; its End is not known yet.
func (d *Decoder) Showing() (Cue, bool) {
	return d.cue, d.open
}

// Close ends the cue on screen at pts.
func (d *Decoder) Close(pts int64) {
	d.end(pts)
}

// due shows typed text that waited out shownEvery.
func (d *Decoder) due(pts int64) {
	if d.waiting && !d.soon(pts) {
		d.typedAt = pts
		d.show(pts)
	}
}

// soon is true within shownEvery of the last showing of typed text. A clock
// that went back is not soon.
func (d *Decoder) soon(pts int64) bool {
	delta := ptsDelta(pts, d.typedAt)
	return delta >= 0 && delta < shownEvery
}

func (d *Decoder) pair(pts int64, b1, b2 byte) {
	if b1 == 0 && b2 == 0 {
		return
	}
	if b1 >= 0x10 && b1 <= 0x1f {
		if d.waiting {
			d.show(pts)
		}
		// Control codes are sent twice so one lost pair doesn't lose the command.
		if d.last == [2]byte{b1, b2} {
			d.last = [2]byte{}
			return
		}
		d.last = [2]byte{b1, b2}
		d.channel = 1
		if b1&0x08 != 0 {
			d.channel = 2
		}
		if d.channel == 1 {
			d.control(pts, b1&^0x08, b2)
		}
		return
	}
	d.last = [2]byte{}
	if d.channel != 1 || d.text || b1 < 0x20 {
		return
	}
	d.put(basic(b1))
	if b2 >= 0x20 {
		d.put(basic(b2))
	}
	d.touch(pts)
}

func (d *Decoder) control(pts int64, b1, b2 byte) {
	switch {
	case (b1 == 0x14 || b1 == 0x15) && b2 >= 0x20 && b2 <= 0x2f:
		d.misc(pts, b2)
	case b1 == 0x17 && b2 >= 0x21 && b2 <= 0x23:
		d.col = min(d.col+int(b2-0x20), cols-1)
	case b2 >= 0x40 && b2 <= 0x7f:
		d.pac(b1, b2)
	case d.text:
	case b1 == 0x11 && b2 >= 0x20 && b2 <= 0x2f:
		// A mid-row style change takes one cell and shows as a space.
		d.put(' ')
		d.touch(pts)
	case b1 == 0x11 && b2 >= 0x30 && b2 <= 0x3f:
		d.put(special[b2-0x30])
		d.touch(pts)
	case (b1 == 0x12 || b1 == 0x13) && b2 >= 0x20 && b2 <= 0x3f:
		// Extended characters replace the standard one sent just before them.
		d.col = max(d.col-1, 0)
		if b1 == 0x12 {
			d.put(extended12[b2-0x20])
		} else {
			d.put(extended13[b2-0x20])
		}
		d.touch(pts)
	}
}

func (d *Decoder) misc(pts int64, b2 byte) {
	switch b2 {
	case 0x20: // RCL resume caption loading
		d.mode, d.text = popOn, false
	case 0x21: // BS backspace
		if d.col > 0 {
			d.col--
			d.target()[d.row][d.col] = 0
			d.touch(pts)
		}
	case 0x24: // DER delete to end of row
		for c := d.col; c < cols; c++ {
			d.target()[d.row][c] = 0
		}
		d.touch(pts)
	case 0x25, 0x26, 0x27: // RU2, RU3, RU4
		if d.mode != rollUp {
			d.shown.clear()
			d.hidden.clear()
			d.row = rows - 1
			d.show(pts)
		}
		d.mode, d.text = rollUp, false
		d.depth = int(b2-0x25) + 2
		d.col = 0
		d.typed = false
	case 0x29: // RDC resume direct captioning
		d.mode, d.text = paintOn, false
		d.typed = false
	case 0x2a, 0x2b: // TR, RTD: the text service, not captions
		d.text = true
	case 0x2c: // EDM erase displayed memory
		d.shown.clear()
		d.show(pts)
	case 0x2d: // CR carriage return
		if d.mode != rollUp {
			return
		}
		d.show(pts)
		top := max(d.row-d.depth+1, 0)
		for r := top; r < d.row; r++ {
			d.shown[r] = d.shown[r+1]
		}
		d.shown[d.row] = [cols]rune{}
		for r := 0; r < top; r++ {
			d.shown[r] = [cols]rune{}
		}
		d.col = 0
		// A TV scrolls now; a row that rolled off the top leaves now.
		d.show(pts)
		d.typed = false
	case 0x2e: // ENM erase non-displayed memory
		d.hidden.clear()
	case 0x2f: // EOC end of caption
		d.shown, d.hidden = d.hidden, d.shown
		d.mode, d.text = popOn, false
		d.show(pts)
	}
}

var pacRows = map[byte][2]int{
	0x11: {0, 1}, 0x12: {2, 3}, 0x15: {4, 5}, 0x16: {6, 7}, 0x17: {8, 9},
	0x10: {10, 10}, 0x13: {11, 12}, 0x14: {13, 14},
}

func (d *Decoder) pac(b1, b2 byte) {
	pair, ok := pacRows[b1]
	if !ok {
		return
	}
	row := pair[0]
	if b2&0x20 != 0 {
		row = pair[1]
	}
	if d.mode == rollUp && row != d.row {
		// The roll-up window moves with its base row.
		moved := screen{}
		for i := 0; i < d.depth; i++ {
			from, to := d.row-i, row-i
			if from >= 0 && to >= 0 {
				moved[to] = d.shown[from]
			}
		}
		d.shown = moved
	}
	d.row = row
	d.col = 0
	if b2&0x10 != 0 {
		d.col = int(b2&0x0e) >> 1 * 4
	}
}

// target is the memory text goes to: hidden while loading a pop-on caption.
func (d *Decoder) target() *screen {
	if d.mode == popOn {
		return &d.hidden
	}
	return &d.shown
}

func (d *Decoder) put(r rune) {
	d.target()[d.row][d.col] = r
	if d.col < cols-1 {
		d.col++
	}
}

// touch notes a change to the screen. A pop-on caption changes the hidden
// memory, which shows at EOC. Typed text shows at once, then at most every
// shownEvery.
func (d *Decoder) touch(pts int64) {
	if d.mode == popOn || d.waiting {
		return
	}
	if d.typed && d.soon(pts) {
		d.waiting = true
		return
	}
	d.typed, d.typedAt = true, pts
	d.show(pts)
}

// show makes what is on screen now the current cue.
func (d *Decoder) show(pts int64) {
	d.waiting = false
	text, top := render(&d.shown)
	if d.open && text == d.cue.Text {
		return
	}
	d.end(pts)
	if text == "" {
		return
	}
	d.open = true
	d.cue = Cue{Start: pts, Text: text, Top: top}
}

func (d *Decoder) end(pts int64) {
	if !d.open {
		return
	}
	d.open = false
	if ptsDelta(pts, d.cue.Start) <= 0 {
		return
	}
	d.cue.End = pts
	d.done = append(d.done, d.cue)
}

func render(s *screen) (string, bool) {
	var lines []string
	first := -1
	for r := range s {
		var b strings.Builder
		for _, c := range s[r] {
			if c == 0 {
				c = ' '
			}
			b.WriteRune(c)
		}
		line := strings.TrimSpace(b.String())
		if line == "" {
			continue
		}
		if first < 0 {
			first = r
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), first >= 0 && first < rows/2
}

func basic(b byte) rune {
	switch b {
	case 0x27:
		return '’'
	case 0x2a:
		return 'á'
	case 0x5c:
		return 'é'
	case 0x5e:
		return 'í'
	case 0x5f:
		return 'ó'
	case 0x60:
		return 'ú'
	case 0x7b:
		return 'ç'
	case 0x7c:
		return '÷'
	case 0x7d:
		return 'Ñ'
	case 0x7e:
		return 'ñ'
	case 0x7f:
		return '█'
	}
	return rune(b)
}

var special = []rune("®°½¿™¢£♪à èâêîôû")

var extended12 = []rune("ÁÉÓÚÜü‘¡*'—©℠•“”ÀÂÇÈÊËëÎÏïÔÙùÛ«»")

var extended13 = []rune("ÃãÍÌìÒòÕõ{}\\^_|~ÄäÖöß¥¤¦ÅåØø┌┐└┘")
