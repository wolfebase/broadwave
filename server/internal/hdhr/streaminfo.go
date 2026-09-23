package hdhr

import (
	"strconv"
	"strings"
)

// Program is one line of tuner streaminfo: "3: 9.1 KMBC-HD".
type Program struct {
	Number      int
	GuideNumber string
	Name        string
}

// ParseStreamInfo reads the HDHomeRun streaminfo document.
func ParseStreamInfo(text string) []Program {
	var out []Program
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		num, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(num))
		if err != nil {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		name := ""
		if len(fields) > 1 {
			name = strings.Join(fields[1:], " ")
		}
		out = append(out, Program{Number: n, GuideNumber: fields[0], Name: name})
	}
	return out
}

// FrequencyHz reads 563000000 from a status string like "ch=8vsb:563000000 lock=8vsb".
func FrequencyHz(status string) int {
	_, after, ok := strings.Cut(status, "ch=")
	if !ok {
		return 0
	}
	field := strings.Fields(after)
	if len(field) == 0 {
		return 0
	}
	parts := strings.Split(field[0], ":")
	n, _ := strconv.Atoi(parts[len(parts)-1])
	if n < 50000000 {
		return 0
	}
	return n
}
