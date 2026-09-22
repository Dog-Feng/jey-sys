package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jev-sys/bot/internal/api"
	"github.com/jev-sys/bot/internal/config"
	"github.com/jev-sys/bot/internal/exchange"
	"github.com/jev-sys/bot/internal/model"
	"github.com/jev-sys/bot/internal/store"
	"github.com/jev-sys/bot/internal/trader"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	ex, err := exchange.New(cfg)
	if err != nil {
		logger.Error("exchange", "err", err)
		os.Exit(1)
	}

	var m model.Model
	switch cfg.Model {
	case "mock":
		m = model.NewMock()
	case "jev":
		m = model.NewJev(cfg)
	default:
		logger.Error("unknown MODEL", "model", cfg.Model)
		os.Exit(1)
	}

	st, err := store.NewJSONL(cfg.DataDir)
	if err != nil {
		logger.Error("store", "err", err)
		os.Exit(1)
	}

	tr := trader.New(cfg, ex, m, st, logger)
	srv := api.New(tr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("http listen", "addr", cfg.HTTPListen)
		if err := http.ListenAndServe(cfg.HTTPListen, srv.Handler()); err != nil && err != http.ErrServerClosed {
			logger.Error("http", "err", err)
			stop()
		}
	}()

	startLog := []any{
		"exchange", cfg.Exchange,
		"dry_run", cfg.DryRun,
		"model", cfg.Model,
		"symbol", cfg.Symbol,
		"tick", cfg.TickInterval.String(),
		"cwd", mustCWD(),
	}
	if cfg.Exchange == "lighter" && cfg.LighterLeverage > 0 {
		startLog = append(startLog, "lighter_leverage", cfg.LighterLeverage, "lighter_leverage_cross", cfg.LighterLeverageCross)
	}
	logger.Info("bot start", startLog...)

	if err := tr.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("run", "err", err)
		os.Exit(1)
	}
	logger.Info("bot stopped")
}

func mustCWD() string {
	dir, err := os.Getwd()
	if err != nil {
		return "?"
	}
	return dir
}
