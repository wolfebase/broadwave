package sports

import (
	"context"
	"time"
)

// Off answers the scoreboard without calling anyone. Live scores are turned off.
type Off struct{}

func (Off) Scoreboard(context.Context, string, time.Time) ([]Game, error) {
	return nil, nil
}

func (Off) Boards(context.Context, time.Time) ([]Game, error) {
	return nil, nil
}
