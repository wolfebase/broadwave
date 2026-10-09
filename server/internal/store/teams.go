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

// sqlExec is the connection a team and its pass share, so one failure undoes both.
type sqlExec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// FollowTeam saves a team. Record every game creates a team pass and turning it off removes that pass.
func (s *Store) FollowTeam(ctx context.Context, team TeamFollow) error {
	team.Name = strings.TrimSpace(team.Name)
	team.Short = strings.TrimSpace(team.Short)
	team.League = strings.TrimSpace(team.League)
	if team.Name == "" {
		return errTeamName
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM team_follows WHERE league = ? AND name = ?`, team.League, team.Name).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `
UPDATE team_follows SET short_name = ?, abbr = ?, logo = ?, color = ?, record = ? WHERE id = ?`,
			team.Short, team.Abbr, team.Logo, team.Color, bit(team.Record), id)
	} else {
		_, err = tx.ExecContext(ctx, `
INSERT INTO team_follows (name, short_name, abbr, league, logo, color, record)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			team.Name, team.Short, team.Abbr, team.League, team.Logo, team.Color, bit(team.Record))
	}
	if err != nil {
		return err
	}
	if err := syncTeamPass(ctx, tx, team); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UnfollowTeam(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var name, short string
	err = tx.QueryRowContext(ctx, `SELECT name, short_name FROM team_follows WHERE id = ?`, id).Scan(&name, &short)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM team_follows WHERE id = ?`, id); err != nil {
		return err
	}
	if err := deleteTeamPass(ctx, tx, name, short); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetTeamNotice(ctx context.Context, id int64, notice string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE team_follows SET last_notice = ? WHERE id = ?`, notice, id)
	return err
}

func syncTeamPass(ctx context.Context, db sqlExec, team TeamFollow) error {
	label := teamPassLabel(team)
	if !team.Record || label == "" {
		return deleteTeamPass(ctx, db, team.Name, team.Short)
	}
	// A pass already under this label keeps its rank and rules.
	var others []string
	for _, name := range []string{team.Name, team.Short} {
		if !strings.EqualFold(strings.TrimSpace(name), label) {
			others = append(others, name)
		}
	}
	if err := deleteTeamPass(ctx, db, others...); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO passes (title, channel_id, kind, pad_before, pad_after, match_kind)
SELECT ?, 0, 'team', 1, 2, 'team'
WHERE NOT EXISTS (SELECT 1 FROM passes WHERE kind = 'team' AND lower(title) = lower(?))`, label, label)
	return err
}

func deleteTeamPass(ctx context.Context, db sqlExec, names ...string) error {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM passes WHERE kind = 'team' AND lower(title) = lower(?)`, name); err != nil {
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
