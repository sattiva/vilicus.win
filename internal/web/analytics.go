package web

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	days := r.URL.Query().Get("days")
	switch days {
	case "7", "90":
	default:
		days = "30"
	}
	mod := "-" + days + " days"
	ctx := r.Context()

	sum, err := s.Store.UsageSummary(ctx, mod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cmds, _ := s.Store.TopCommands(ctx, mod, 10)
	users, _ := s.Store.TopUsers(ctx, mod, 10)
	guilds, _ := s.Store.TopGuilds(ctx, mod, 10)
	hours, _ := s.Store.UsageByHour(ctx, mod)
	audit, _ := s.Store.ListDashAudit(ctx, 25)

	var peak int64
	for _, c := range hours {
		if c > peak {
			peak = c
		}
	}
	type hourBar struct {
		Hour  int
		Count int64
		Pct   float64
	}
	bars := make([]hourBar, 24)
	for i, c := range hours {
		pct := 0.0
		if peak > 0 {
			pct = float64(c) * 100 / float64(peak)
		}
		bars[i] = hourBar{Hour: i, Count: c, Pct: pct}
	}

	errorRate := 0.0
	if sum.Total > 0 {
		errorRate = float64(sum.Errors) * 100 / float64(sum.Total)
	}

	hist, _ := s.Store.LatencyHistogram(ctx, mod)

	type gauge struct {
		Label string
		Value string
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	dbSize, _ := s.Store.SizeBytes(ctx)

	uptime := "-"
	guilds2 := 0
	heartbeat := int64(0)
	if s.Bot != nil {
		uptime = time.Since(s.Bot.StartTime).Truncate(time.Second).String()
		if s.Bot.Session != nil {
			heartbeat = s.Bot.Session.HeartbeatLatency().Milliseconds()
			s.Bot.Session.State.RLock()
			guilds2 = len(s.Bot.Session.State.Guilds)
			s.Bot.Session.State.RUnlock()
		}
	}
	dbMB := fmt.Sprintf("%.2f MB", float64(dbSize)/1024/1024)
	heapMB := fmt.Sprintf("%.1f MB", float64(m.HeapAlloc)/1024/1024)
	sysMB := fmt.Sprintf("%.1f MB", float64(m.Sys)/1024/1024)

	process := []gauge{
		{"Uptime", uptime},
		{"Heartbeat", fmt.Sprintf("%d ms", heartbeat)},
		{"Guilds (live)", strconv.Itoa(guilds2)},
		{"Heap Alloc", heapMB},
		{"Heap Sys", sysMB},
		{"Goroutines", strconv.Itoa(runtime.NumGoroutine())},
		{"DB Size", dbMB},
		{"Writer Queue / Dropped", fmt.Sprintf("%d / %d", s.Store.QueueDepth(), s.Store.DroppedCommands())},
		{"Rate-limited (global / auth / write)", fmt.Sprintf("%d / %d / %d", s.rejectedGlobal.Load(), s.rejectedAuth.Load(), s.rejectedWrite.Load())},
	}

	s.render(w, r, "analytics", "Analytics", "analytics", map[string]any{
		"Days":        days,
		"Summary":     sum,
		"ErrorRate":   fmt.Sprintf("%.1f", errorRate),
		"AvgLatency":  fmt.Sprintf("%.1f ms", sum.AvgLatency),
		"Histogram":   hist,
		"Process":     process,
		"TopCommands": cmds,
		"TopUsers":    users,
		"TopGuilds":   guilds,
		"Hours":       bars,
		"Audit":       audit,
	})
}

