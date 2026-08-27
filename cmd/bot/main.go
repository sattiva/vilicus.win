package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vilicus/internal/config"
	"vilicus/internal/discord"
	"vilicus/internal/logging"
	"vilicus/internal/store"
	"vilicus/internal/web"
)

func main() {
	cfg := config.Load()
	l := logging.Setup(cfg.LogLevel, cfg.LogFormat, cfg.LogPath)

	l.Info("starting vilicus", "log_level", cfg.LogLevel, "log_format", cfg.LogFormat)

	if cfg.PprofEnabled {
		go func() {
			_ = http.ListenAndServe("localhost:6060", nil)
		}()
		l.Info("pprof profiling enabled", "addr", "localhost:6060")
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		l.Error("failed opening database", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	var b *discord.Bot
	if cfg.BotToken != "" {
		var bErr error
		b, bErr = discord.New(cfg, st)
		if bErr != nil {
			l.Error("failed initializing discord bot", "err", bErr)
			os.Exit(1)
		}

		if err := b.Start(); err != nil {
			l.Error("failed starting discord gateway", "err", err)
		}
	} else {
		l.Warn("discord bot token not set, running web dashboard only")
	}

	srv, err := web.New(cfg, st, b)
	if err != nil {
		l.Error("failed creating web server", "err", err)
		os.Exit(1)
	}

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      srv,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		l.Info("web server listening", "port", cfg.HTTPPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			l.Error("web server error", "err", err)
		}
	}()

	ticker := time.NewTicker(time.Hour * 6)
	defer ticker.Stop()

	latencyTicker := time.NewTicker(5 * time.Minute)
	defer latencyTicker.Stop()
	logLatency := func() {
		p50, p95, ok, err := st.LatencyPercentiles(context.Background(), 5*time.Minute)
		if err != nil {
			l.Warn("latency percentile query failed", "err", err)
			return
		}
		if ok {
			l.Info("command latency percentiles (5m window)", "p50_ms", p50, "p95_ms", p95)
		}
	}

	stopHousekeeping := make(chan struct{})
	go func() {
		lastBackupDay := ""
		runBackup := func() {
			day := time.Now().Format("2006-01-02")
			if day == lastBackupDay {
				return
			}
			dest, err := st.RunBackupCycle(context.Background(), cfg.BackupDir, 7, 4)
			if err != nil {
				l.Warn("nightly backup failed", "err", err)
				return
			}
			l.Info("nightly backup created", "path", dest)
			lastBackupDay = day
		}
		runBackup()

		for {
			select {
			case <-latencyTicker.C:
				logLatency()
			case <-ticker.C:
				l.Info("running database maintenance")
				if err := st.Prune(cfg.RetentionDays, cfg.RetentionAuditDays); err != nil {
					l.Warn("pruning failed", "err", err)
				}
				if err := st.Checkpoint(); err != nil {
					l.Warn("wal checkpoint failed", "err", err)
				}
				if err := st.Analyze(); err != nil {
					l.Warn("analyze failed", "err", err)
				}
				runBackup()
			case <-stopHousekeeping:
				return
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	l.Info("shutting down vilicus")
	close(stopHousekeeping)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = httpSrv.Shutdown(ctx)
	srv.Close()

	if b != nil {
		b.Stop()
	}
	st.Stop()

	l.Info("shutdown complete")
}

