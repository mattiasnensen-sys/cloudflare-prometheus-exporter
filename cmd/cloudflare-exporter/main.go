package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mattiasnensen-sys/cloudflare-exporter/internal/cloudflare"
	"github.com/mattiasnensen-sys/cloudflare-exporter/internal/config"
	"github.com/mattiasnensen-sys/cloudflare-exporter/internal/exporter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("starting cloudflare exporter",
		"poll_interval", cfg.PollInterval.String(),
		"window_duration", cfg.WindowDuration.String(),
		"scrape_delay", cfg.ScrapeDelay.String(),
		"zone_count", len(cfg.CloudflareZoneTags),
		"graphql_endpoint", cfg.CloudflareGraphQL,
	)

	cfClient := cloudflare.NewClient(
		cfg.CloudflareGraphQL,
		cfg.CloudflareAPIToken,
		cfg.QueryLimit,
		cfg.RequestTimeout,
	)

	metrics, registry := exporter.NewMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runPoller(ctx, cfClient, metrics, cfg)

	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc(cfg.HealthPath, func(w http.ResponseWriter, r *http.Request) {
		state := metrics.Health()
		status := http.StatusOK
		if !state.LastPollTime.IsZero() && !state.LastPollTime.After(time.Now().UTC().Add(-3*cfg.PollInterval)) {
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(state)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "cloudflare-exporter running\nmetrics: %s\nhealth: %s\n", cfg.MetricsPath, cfg.HealthPath)
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server failed", "error", err)
			cancel()
		}
	}()

	slog.Info("http server listening", "addr", server.Addr)

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()

	slog.Info("shutdown signal received")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown failed", "error", err)
		os.Exit(1)
	}
}

func runPoller(ctx context.Context, cfClient *cloudflare.Client, metrics *exporter.Metrics, cfg config.Config) {
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	poll := func() {
		now := time.Now().UTC()
		windowEnd := now.Add(-cfg.ScrapeDelay)
		windowStart := windowEnd.Add(-cfg.WindowDuration)

		start := time.Now()
		result, err := cfClient.FetchMetrics(ctx, cfg.CloudflareZoneTags, cloudflare.RequestWindow{
			MinTime: windowStart,
			MaxTime: windowEnd,
		})
		duration := time.Since(start)

		if err != nil {
			metrics.ObserveError(duration, err)
			slog.Warn("poll failed", "error", err, "window_start", windowStart, "window_end", windowEnd)
			return
		}

		zoneTags := make([]string, 0, len(result.Zones))
		for _, zone := range result.Zones {
			zoneTags = append(zoneTags, zone.ZoneTag)
		}
		if len(zoneTags) == 0 {
			zoneTags = append(zoneTags, cfg.CloudflareZoneTags...)
		}

		metrics.Ingest(result)
		metrics.ObserveSuccess(duration, zoneTags)
		slog.Info("poll completed", "zones", len(result.Zones), "workers_samples", len(result.Workers), "duration", duration.String(), "window_start", windowStart, "window_end", windowEnd)
	}

	poll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}
