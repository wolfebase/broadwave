package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var errTeamName = errors.New("a team needs a name")

// TeamFollow is a team this household wants on the home screen.
type TeamFollow struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Short      string `json:"short,omitempty"`
	Abbr       string `json:"abbr,omitempty"`
	League     string `json:"league,omitempty"`
	Logo       string `json:"logo,omitempty"`
	Color      string `json:"color,omitempty"`
	Record     bool   `json:"record,omitempty"`
	LastNotice string `json:"-"`
}

func (s *Store) TeamFollows(ctx context.Context) ([]TeamFollow, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, short_name, abbr, league, logo, color, record, last_notice
FROM team_follows ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeamFollow
	for rows.Next() {
		var team TeamFollow
		var record int
		if err := rows.Scan(&team.ID, &team.Name, &team.Short, &team.Abbr, &team.League, &team.Logo, &team.Color, &record, &team.LastNotice); err != nil {
			return nil, err
		}
		team.Record = record != 0
		out = append(out, team)
	}
	if out == nil {
		out = []TeamFollow{}
	}
	return out, rows.Err()
}

// FollowTeam saves a team. Record every game creates a team pass and turning it off removes that pass.
func (s *Store) FollowTeam(ctx context.Context, team TeamFollow) error {
	team.Name = strings.TrimSpace(team.Name)
	team.Short = strings.TrimSpace(team.Short)
	team.League = strings.TrimSpace(team.League)
	if team.Name == "" {
		return errTeamName
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM team_follows WHERE league = ? AND name = ?`, team.League, team.Name).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		_, err = s.db.ExecContext(ctx, `
UPDATE team_follows SET short_name = ?, abbr = ?, logo = ?, color = ?, record = ? WHERE id = ?`,
			team.Short, team.Abbr, team.Logo, team.Color, bit(team.Record), id)
	} else {
		_, err = s.db.ExecContext(ctx, `
INSERT INTO team_follows (name, short_name, abbr, league, logo, color, record)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			team.Name, team.Short, team.Abbr, team.League, team.Logo, team.Color, bit(team.Record))
	}
	if err != nil {
		return err
	}
	return s.syncTeamPass(ctx, team)
}

func (s *Store) UnfollowTeam(ctx context.Context, id int64) error {
	var name, short string
	err := s.db.QueryRowContext(ctx, `SELECT name, short_name FROM team_follows WHERE id = ?`, id).Scan(&name, &short)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM team_follows WHERE id = ?`, id); err != nil {
		return err
	}
	return s.deleteTeamPass(ctx, name, short)
}

func (s *Store) SetTeamNotice(ctx context.Context, id int64, notice string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE team_follows SET last_notice = ? WHERE id = ?`, notice, id)
	return err
}

func (s *Store) syncTeamPass(ctx context.Context, team TeamFollow) error {
	label := teamPassLabel(team)
	if err := s.deleteTeamPass(ctx, team.Name, team.Short); err != nil {
		return err
	}
	if !team.Record || label == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO passes (title, channel_id, kind, pad_before, pad_after, match_kind)
VALUES (?, 0, 'team', 1, 2, 'team')`, label)
	return err
}

func (s *Store) deleteTeamPass(ctx context.Context, names ...string) error {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM passes WHERE kind = 'team' AND lower(title) = lower(?)`, name); err != nil {
			return err
		}
	}
	return nil
}

func teamPassLabel(team TeamFollow) string {
	if len([]rune(strings.TrimSpace(team.Short))) >= 4 {
		return strings.TrimSpace(team.Short)
	}
	return strings.TrimSpace(team.Name)
}

func bit(v bool) int {
	if v {
		return 1
	}
	return 0
}
