package psip

import (
	"strings"
	"time"
	"unicode"
)

// Span is one listing on a channel, used to decide what the broadcast may fill.
type Span struct {
	ChannelID int64
	Start     time.Time
	End       time.Time
}

// FillGaps returns the extra listings that do not overlap one already held
// on the same channel. A listing that only touches an edge is kept.
func FillGaps(have, extra []Span) []Span {
	var out []Span
	for _, row := range extra {
		if row.End.After(row.Start) && !overlaps(have, row) {
			out = append(out, row)
		}
	}
	return out
}

func overlaps(have []Span, row Span) bool {
	for _, got := range have {
		if got.ChannelID != row.ChannelID {
			continue
		}
		if row.Start.Before(got.End) && got.Start.Before(row.End) {
			return true
		}
	}
	return false
}

// TitleCase turns an ALL-CAPS title into title case and leaves mixed case alone.
func TitleCase(title string) string {
	letters := 0
	upper := 0
	for _, r := range title {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters == 0 || upper*2 < letters {
		return title
	}
	lower := strings.ToLower(title)
	var b strings.Builder
	cap := true
	for _, r := range lower {
		if cap && unicode.IsLetter(r) {
			b.WriteRune(unicode.ToUpper(r))
			cap = false
			continue
		}
		b.WriteRune(r)
		cap = !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}
	return b.String()
}
