package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestAnalyticsAggregates(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	batch := []CommandLog{
		{CommandName: "ban", GuildID: "g1", UserID: "u1", Status: "success", ExecutionMS: 100, CreatedAt: now.Add(-time.Hour)},
		{CommandName: "ban", GuildID: "g1", UserID: "u2", Status: "success", ExecutionMS: 200, CreatedAt: now.Add(-2 * time.Hour)},
		{CommandName: "ban", GuildID: "g2", UserID: "u1", Status: "error", ExecutionMS: 300, CreatedAt: now.Add(-3 * time.Hour)},
		{CommandName: "roll", UserID: "u3", Status: "success", ExecutionMS: 10, CreatedAt: now.Add(-4 * time.Hour)},
	}
	st.insertCommandLogs(batch)

	cmds, err := st.TopCommands(ctx, "-30 days", 10)
	if err != nil {
		t.Fatalf("top commands: %v", err)
	}
	if len(cmds) != 2 || cmds[0].Name != "ban" || cmds[0].Count != 3 {
		t.Fatalf("want ban=3 first, got %+v", cmds)
	}

	users, _ := st.TopUsers(ctx, "-30 days", 10)
	if len(users) != 3 || users[0].Name != "u1" || users[0].Count != 2 {
		t.Fatalf("want u1=2 first, got %+v", users)
	}

	guilds, _ := st.TopGuilds(ctx, "-30 days", 10)
	if len(guilds) != 2 {
		t.Fatalf("want 2 guilds (empty skipped), got %+v", guilds)
	}

	sum, err := st.UsageSummary(ctx, "-30 days")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if sum.Total != 4 || sum.Errors != 1 || sum.AvgLatency != 152.5 {
		t.Fatalf("summary = %+v, want total=4 errors=1 avg=152.5", sum)
	}

	old, _ := st.TopCommands(ctx, "+30 days", 10)
	if len(old) != 0 {
		t.Fatalf("future window should be empty, got %+v", old)
	}
}

func TestLatencyPercentiles(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	batch := make([]CommandLog, 0, 100)
	for i := 0; i < 100; i++ {
		batch = append(batch, CommandLog{
			CommandName: "roll", UserID: "u1", Status: "success",
			ExecutionMS: int64(i + 1),
			AckMS:       -1, SendMS: int64(i + 1),
			CreatedAt: now,
		})
	}
	st.insertCommandLogs(batch)

	p50, p95, ok, err := st.LatencyPercentiles(ctx, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("percentiles: %v", err)
	}
	if !ok {
		t.Fatal("window has samples but ok=false")
	}
	if p50 != 50 || p95 != 95 {
		t.Fatalf("p50=%v p95=%v, want 50/95", p50, p95)
	}

	empty := openTestStore(t)
	_, _, ok, err = empty.LatencyPercentiles(ctx, time.Hour)
	if err != nil || ok {
		t.Fatalf("empty store: ok=%v err=%v, want false/nil", ok, err)
	}
}

func TestLogCommandSpanPersistence(t *testing.T) {
	st := openTestStore(t)

	waitSpans := func(t *testing.T, cmd string) (ack, send sql.NullInt64) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			err := st.db.QueryRow(`SELECT ack_ms, send_ms FROM command_usage_log WHERE command_name = ?`, cmd).Scan(&ack, &send)
			if err == nil {
				return ack, send
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("usage row for %s never appeared", cmd)
		return ack, send
	}

	if err := st.LogCommand(context.Background(), "ban", "g1", "u1", "success", 250, Spans{AckMS: 80, SendMS: 240}); err != nil {
		t.Fatalf("log: %v", err)
	}
	ack, send := waitSpans(t, "ban")
	if !ack.Valid || ack.Int64 != 80 || !send.Valid || send.Int64 != 240 {
		t.Fatalf("spans = %v/%v, want 80/240", ack, send)
	}

	if err := st.LogCommand(context.Background(), "roll", "", "u2", "success", 5); err != nil {
		t.Fatalf("log: %v", err)
	}
	ack, send = waitSpans(t, "roll")
	if ack.Valid || send.Valid {
		t.Fatalf("unmeasured spans stored as %v/%v, want NULL/NULL", ack, send)
	}
}

