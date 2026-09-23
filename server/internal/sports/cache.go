package sports

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	liveTTL    = 30 * time.Second
	quietTTL   = 2 * time.Hour
	minBackoff = 30 * time.Second
	maxBackoff = 30 * time.Minute
)

// Cache keeps a scoreboard and asks the provider again only when it is stale.
// A league with a game in progress refreshes every 30 seconds. A quiet board
// waits two hours. A failed fetch backs off and serves the last good board.
type Cache struct {
	Provider Provider
	now      func() time.Time

	mu    sync.Mutex
	items map[string]cached
	wait  map[string]time.Time
	delay map[string]time.Duration
}

type cached struct {
	games []Game
	at    time.Time
	ttl   time.Duration
}

func NewCache(provider Provider) *Cache {
	return &Cache{Provider: provider, now: time.Now, items: map[string]cached{}, wait: map[string]time.Time{}, delay: map[string]time.Duration{}}
}

func (c *Cache) Scoreboard(ctx context.Context, league string, day time.Time) ([]Game, error) {
	key := league + ":" + day.Format("20060102")
	now := c.now()
	c.mu.Lock()
	item, ok := c.items[key]
	fresh := ok && now.Sub(item.at) < item.ttl
	cooling := now.Before(c.wait[key])
	c.mu.Unlock()
	if cooling {
		if ok {
			return item.games, nil
		}
		return nil, fmt.Errorf("scoreboard is waiting to retry")
	}
	if fresh {
		return item.games, nil
	}
	games, err := c.Provider.Scoreboard(ctx, league, day)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		delay := c.delay[key]
		if delay < minBackoff {
			delay = minBackoff
		} else {
			delay *= 2
			if delay > maxBackoff {
				delay = maxBackoff
			}
		}
		c.delay[key] = delay
		c.wait[key] = c.now().Add(delay)
		if ok {
			return item.games, nil
		}
		return nil, err
	}
	delete(c.delay, key)
	delete(c.wait, key)
	ttl := quietTTL
	for _, game := range games {
		if game.Live() {
			ttl = liveTTL
			break
		}
	}
	c.items[key] = cached{games: games, at: c.now(), ttl: ttl}
	return games, nil
}

// Boards reads every known league for the day. A league that fails is skipped
// when another league still has games.
func (c *Cache) Boards(ctx context.Context, day time.Time) ([]Game, error) {
	var out []Game
	var first error
	for _, league := range Leagues {
		games, err := c.Scoreboard(ctx, league.ID, day)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		out = append(out, games...)
	}
	if len(out) == 0 && first != nil {
		return nil, first
	}
	if out == nil {
		out = []Game{}
	}
	return out, nil
}
