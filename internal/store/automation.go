package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)


type AutomationRule struct {
	ID              int64      `json:"id"`
	GuildID         string     `json:"guild_id"`
	Name            string     `json:"name"`
	Enabled         bool       `json:"enabled"`
	Trigger         string     `json:"trigger"`
	TriggerArg      string     `json:"trigger_arg"`
	Channels        string     `json:"channels"`
	Actors          string     `json:"actors"`
	MinAccountAge   int64      `json:"min_account_age_seconds"`
	RequireRoles    string     `json:"require_roles"`
	ForbidRoles     string     `json:"forbid_roles"`
	Phrases         string     `json:"phrases"`
	Pattern         string     `json:"pattern"`
	Links           bool       `json:"links"`
	MinMentions     int        `json:"min_mentions"`
	CooldownSeconds int64      `json:"cooldown_seconds"`
	CounterLimit    int        `json:"counter_limit"`
	CounterWindow   int64      `json:"counter_window_seconds"`
	Actions         string     `json:"actions"`
	Template        string     `json:"template"`
	LastRun         *time.Time `json:"-"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
}

var ValidAutomationTriggers = map[string]bool{
	"message_create": true,
	"member_join":    true,
	"member_leave":   true,
	"member_ban":     true,
	"member_unban":   true,
	"role_add":       true,
	"role_remove":    true,
	"interval":       true,
}

const automationCols = `id, guild_id, name, enabled, trigger, trigger_arg, channels, actors,
	min_account_age, require_roles, forbid_roles, phrases, pattern, links, min_mentions,
	cooldown_seconds, counter_limit, counter_window, actions, template, last_run, created_by, created_at`

func scanAutomation(sc interface{ Scan(...any) error }) (*AutomationRule, error) {
	r := &AutomationRule{}
	var enabled int
	var lastRun sql.NullTime
	err := sc.Scan(&r.ID, &r.GuildID, &r.Name, &enabled, &r.Trigger, &r.TriggerArg, &r.Channels,
		&r.Actors, &r.MinAccountAge, &r.RequireRoles, &r.ForbidRoles, &r.Phrases, &r.Pattern,
		&r.Links, &r.MinMentions, &r.CooldownSeconds, &r.CounterLimit, &r.CounterWindow,
		&r.Actions, &r.Template, &lastRun, &r.CreatedBy, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	if lastRun.Valid {
		t := lastRun.Time
		r.LastRun = &t
	}
	return r, nil
}

func (s *Store) CreateAutomationRule(ctx context.Context, r *AutomationRule) error {
	if !ValidAutomationTriggers[r.Trigger] {
		return fmt.Errorf("unknown trigger %q", r.Trigger)
	}
	if r.Name == "" || r.GuildID == "" {
		return errors.New("automation rule needs guild and name")
	}
	if r.Actors == "" {
		r.Actors = "any"
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO automation_rules (guild_id, name, enabled, trigger, trigger_arg, channels, actors,
			min_account_age, require_roles, forbid_roles, phrases, pattern, links, min_mentions,
			cooldown_seconds, counter_limit, counter_window, actions, template, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.GuildID, r.Name, boolInt(r.Enabled), r.Trigger, r.TriggerArg, r.Channels, r.Actors,
		r.MinAccountAge, r.RequireRoles, r.ForbidRoles, r.Phrases, r.Pattern, boolInt(r.Links),
		r.MinMentions, r.CooldownSeconds, r.CounterLimit, r.CounterWindow, r.Actions, r.Template,
		r.CreatedBy)
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) ListAutomationRules(ctx context.Context, gid string) ([]AutomationRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+automationCols+` FROM automation_rules WHERE guild_id = ? ORDER BY name`, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutomationRule
	for rows.Next() {
		r, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *Store) GetAutomationRule(ctx context.Context, gid, name string) (*AutomationRule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+automationCols+` FROM automation_rules WHERE guild_id = ? AND name = ?`, gid, name)
	r, err := scanAutomation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

func (s *Store) SetAutomationRuleEnabled(ctx context.Context, gid, name string, enabled bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE automation_rules SET enabled = ? WHERE guild_id = ? AND name = ?`, boolInt(enabled), gid, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("no such rule")
	}
	return nil
}

func (s *Store) DeleteAutomationRule(ctx context.Context, gid, name string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM automation_rules WHERE guild_id = ? AND name = ?`, gid, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("no such rule")
	}
	return nil
}

func (s *Store) DueIntervalRules(ctx context.Context, now time.Time) ([]AutomationRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+automationCols+` FROM automation_rules
		 WHERE trigger = 'interval' AND enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutomationRule
	for rows.Next() {
		r, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		period, ok := parseSeconds(r.TriggerArg)
		if !ok || period <= 0 {
			continue
		}
		last := time.Time{}
		if r.LastRun != nil {
			last = *r.LastRun
		}
		if now.Sub(last) >= time.Duration(period)*time.Second {
			out = append(out, *r)
		}
	}
	return out, rows.Err()
}

func (s *Store) TouchAutomationRuleRun(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE automation_rules SET last_run = ? WHERE id = ?`, time.Now().UTC(), id)
	return err
}

func parseSeconds(s string) (int64, bool) {
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

