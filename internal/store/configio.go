package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)


const BundleVersion = 1

type GuildConfigBundle struct {
	Version         int               `json:"version"`
	ExportedAt      time.Time         `json:"exported_at"`
	GuildID         string            `json:"guild_id"`
	Guild           *GuildConfig      `json:"guild,omitempty"`
	Protection      *ProtectionConfig `json:"protection,omitempty"`
	Starboard       *StarboardConfig  `json:"starboard,omitempty"`
	AutomationRules []AutomationRule  `json:"automation_rules,omitempty"`
}

func (s *Store) ExportGuildBundle(ctx context.Context, gid string) (*GuildConfigBundle, error) {
	b := &GuildConfigBundle{
		Version:    BundleVersion,
		ExportedAt: time.Now().UTC(),
		GuildID:    gid,
	}

	g, err := s.GetGuildConfig(ctx, gid)
	if err != nil {
		return nil, err
	}
	g.UpdatedAt = time.Time{}
	b.Guild = g

	if p, err := s.GetProtectionConfig(ctx, gid); err == nil {
		p.GuildID = ""
		b.Protection = p
	} else if !errors.Is(err, ErrProtectionUnconfigured) {
		return nil, err
	}

	if sb, err := s.GetStarboardConfig(ctx, gid); err == nil {
		sb.GuildID = ""
		b.Starboard = sb
	} else if !errors.Is(err, ErrStarboardDisabled) {
		return nil, err
	}

	rules, err := s.ListAutomationRules(ctx, gid)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		rules[i].GuildID = ""
		rules[i].ID = 0
	}
	b.AutomationRules = rules

	return b, nil
}

func (s *Store) ImportGuildBundle(ctx context.Context, b *GuildConfigBundle, dest string) ([]string, error) {
	if b.Version != BundleVersion {
		return nil, fmt.Errorf("unsupported bundle version %d (want %d)", b.Version, BundleVersion)
	}
	if dest == "" {
		return nil, errors.New("destination guild required")
	}

	for _, r := range b.AutomationRules {
		if r.Name == "" {
			return nil, errors.New("automation rule missing name")
		}
		if !ValidAutomationTriggers[r.Trigger] {
			return nil, fmt.Errorf("automation rule %q has unknown trigger %q", r.Name, r.Trigger)
		}
	}

	var applied []string

	if b.Guild != nil {
		g := *b.Guild
		g.GuildID = dest
		g.UpdatedAt = time.Time{}
		if err := s.SaveGuildConfig(ctx, &g); err != nil {
			return applied, fmt.Errorf("guild section: %w", err)
		}
		applied = append(applied, "guild")
	}

	if b.Protection != nil {
		p := *b.Protection
		p.GuildID = dest
		if err := s.SaveProtectionConfig(ctx, &p); err != nil {
			return applied, fmt.Errorf("protection section: %w", err)
		}
		applied = append(applied, "protection")
	}

	if b.Starboard != nil {
		sb := *b.Starboard
		sb.GuildID = dest
		if err := s.SaveStarboardConfig(ctx, &sb); err != nil {
			return applied, fmt.Errorf("starboard section: %w", err)
		}
		applied = append(applied, "starboard")
	}

	if len(b.AutomationRules) > 0 || s.hasAutomationRules(ctx, dest) {
		existing, err := s.ListAutomationRules(ctx, dest)
		if err != nil {
			return applied, fmt.Errorf("automation section: %w", err)
		}
		for _, r := range existing {
			if err := s.DeleteAutomationRule(ctx, dest, r.Name); err != nil {
				return applied, fmt.Errorf("automation section: removing %q: %w", r.Name, err)
			}
		}
		for _, r := range b.AutomationRules {
			in := r
			in.ID = 0
			in.GuildID = dest
			if err := s.CreateAutomationRule(ctx, &in); err != nil {
				return applied, fmt.Errorf("automation section: creating %q: %w", in.Name, err)
			}
		}
		applied = append(applied, "automation")
	}

	return applied, nil
}

func (s *Store) hasAutomationRules(ctx context.Context, gid string) bool {
	var n int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM automation_rules WHERE guild_id = ?`, gid).Scan(&n)
	return n > 0
}

