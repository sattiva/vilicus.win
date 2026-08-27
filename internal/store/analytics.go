package store

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"
)


type NameCount struct {
	Name  string
	Count int64
}

func (s *Store) topUsage(ctx context.Context, column, days string, limit int) ([]NameCount, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(
		`SELECT %s AS k, COUNT(1) AS c FROM command_usage_log
		 WHERE created_at >= datetime('now', ?) GROUP BY k ORDER BY c DESC LIMIT ?`, column), days, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []NameCount
	for rows.Next() {
		var n NameCount
		if err := rows.Scan(&n.Name, &n.Count); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) TopCommands(ctx context.Context, days string, limit int) ([]NameCount, error) {
	return s.topUsage(ctx, "command_name", days, limit)
}

func (s *Store) TopUsers(ctx context.Context, days string, limit int) ([]NameCount, error) {
	return s.topUsage(ctx, "user_id", days, limit)
}

func (s *Store) TopGuilds(ctx context.Context, days string, limit int) ([]NameCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT guild_id AS k, COUNT(1) AS c FROM command_usage_log
		 WHERE created_at >= datetime('now', ?) AND guild_id != '' GROUP BY k ORDER BY c DESC LIMIT ?`,
		days, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []NameCount
	for rows.Next() {
		var n NameCount
		if err := rows.Scan(&n.Name, &n.Count); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) UsageByHour(ctx context.Context, days string) ([24]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT strftime('%H', created_at) AS h, COUNT(1) FROM command_usage_log
		 WHERE created_at >= datetime('now', ?) GROUP BY h`, days)
	if err != nil {
		return [24]int64{}, err
	}
	defer rows.Close()

	var hours [24]int64
	for rows.Next() {
		var h string
		var c int64
		if err := rows.Scan(&h, &c); err != nil {
			return [24]int64{}, err
		}
		var idx int
		if _, err := fmt.Sscanf(h, "%d", &idx); err != nil || idx < 0 || idx > 23 {
			continue
		}
		hours[idx] = c
	}
	return hours, rows.Err()
}

type UsageSummary struct {
	Total      int64
	Errors     int64
	AvgLatency float64
}

func (s *Store) UsageSummary(ctx context.Context, days string) (UsageSummary, error) {
	var u UsageSummary
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1),
		        COALESCE(SUM(CASE WHEN status != 'success' THEN 1 ELSE 0 END), 0),
		        COALESCE(AVG(execution_ms), 0)
		 FROM command_usage_log WHERE created_at >= datetime('now', ?)`, days).
		Scan(&u.Total, &u.Errors, &u.AvgLatency)
	return u, err
}

func (s *Store) LatencyPercentiles(ctx context.Context, window time.Duration) (p50, p95 float64, ok bool, err error) {
	cutoff := time.Now().UTC().Add(-window)
	rows, err := s.db.QueryContext(ctx,
		`SELECT execution_ms FROM command_usage_log WHERE created_at >= ?`, cutoff)
	if err != nil {
		return 0, 0, false, err
	}
	defer rows.Close()

	var ms []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return 0, 0, false, err
		}
		ms = append(ms, v)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, false, err
	}
	if len(ms) == 0 {
		return 0, 0, false, nil
	}
	slices.Sort(ms)
	pick := func(p float64) float64 {
		idx := max(int(math.Ceil(p/100*float64(len(ms))))-1, 0)
		return float64(ms[idx])
	}
	return pick(50), pick(95), true, nil
}

type LatencyBuckets struct {
	Under10   int64
	B10_25    int64
	B25_50    int64
	B50_100   int64
	B100_250  int64
	B250_500  int64
	B500_1000 int64
	Over1000  int64
}

func (s *Store) LatencyHistogram(ctx context.Context, days string) (LatencyBuckets, error) {
	var b LatencyBuckets
	err := s.db.QueryRowContext(ctx,
		`SELECT
		        COALESCE(SUM(CASE WHEN execution_ms < 10 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN execution_ms >= 10 AND execution_ms < 25 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN execution_ms >= 25 AND execution_ms < 50 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN execution_ms >= 50 AND execution_ms < 100 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN execution_ms >= 100 AND execution_ms < 250 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN execution_ms >= 250 AND execution_ms < 500 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN execution_ms >= 500 AND execution_ms < 1000 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN execution_ms >= 1000 THEN 1 ELSE 0 END), 0)
		 FROM command_usage_log WHERE created_at >= datetime('now', ?)`, days).
		Scan(&b.Under10, &b.B10_25, &b.B25_50, &b.B50_100, &b.B100_250,
			&b.B250_500, &b.B500_1000, &b.Over1000)
	return b, err
}

