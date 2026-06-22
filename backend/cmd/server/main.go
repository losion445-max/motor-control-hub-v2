// Command server runs the robot WebSocket API.
//
// Usage:
//
//	./server -config config.toml
//
// Reads the TOML config file, connects to the servo drives, starts
// broadcasting motor status, and serves a WebSocket endpoint at /ws.
// Ctrl-C triggers a graceful shutdown: the current operation is cancelled
// and all motors are disabled before the process exits.
package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/losion445-max/motor-control-hub-v2/pkg/api"
	appcfg "github.com/losion445-max/motor-control-hub-v2/pkg/config"
	"github.com/losion445-max/motor-control-hub-v2/pkg/robot"
	"github.com/losion445-max/motor-control-hub-v2/pkg/runner"
	"github.com/losion445-max/motor-control-hub-v2/pkg/usecase"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to TOML config file")
	flag.Parse()

	// ── Load config ───────────────────────────────────────────────────────────
	cfg, err := appcfg.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// ── Structured logging ────────────────────────────────────────────────────
	initLogger(cfg.Server.LogFormat, cfg.Server.LogLevel)
	slog.Info("config loaded", "path", *configPath)

	// ── Robot ─────────────────────────────────────────────────────────────────
	robotCfg := robotConfig(cfg)
	sys := robot.NewSystem(cfg.Server.SerialPort, cfg.Server.BaudRate, robotCfg)
	if err := sys.Connect(); err != nil {
		slog.Error("serial connect failed", "port", cfg.Server.SerialPort, "err", err)
		os.Exit(1)
	}
	defer sys.Close()
	slog.Info("drives connected", "port", cfg.Server.SerialPort, "baud", cfg.Server.BaudRate)

	// ── Orchestrator ──────────────────────────────────────────────────────────
	orch := usecase.New(sys)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := time.Duration(cfg.Server.StatusIntervalMs) * time.Millisecond
	go orch.RunStatusBroadcast(ctx, interval)

	// ── HTTP + WebSocket ──────────────────────────────────────────────────────
	gcodeOpts := runner.Opts{
		RapidMmPerSec:       cfg.Gcode.RapidMmPerSec,
		DefaultFeedMmPerSec: cfg.Gcode.DefaultFeedMmPerSec,
	}
	wsHandler := api.NewHandler(orch, gcodeOpts)
	srv := api.NewServer(cfg.Server.Addr, wsHandler)

	go func() {
		slog.Info("server listening",
			"addr", cfg.Server.Addr,
			"ws", "/ws",
			"health", "/health",
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := api.Shutdown(shutCtx, srv); err != nil {
		slog.Warn("http shutdown", "err", err)
	}
	if err := sys.EmergencyStop(); err != nil {
		slog.Warn("emergency stop", "err", err)
	}
	slog.Info("done")
}

// initLogger configures the global slog logger.
func initLogger(format, level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}

// robotConfig maps the loaded app config into robot.Config.
func robotConfig(cfg *appcfg.Config) robot.Config {
	return robot.Config{
		WidthMM:       cfg.Hardware.WidthMM,
		HeightMM:      cfg.Hardware.HeightMM,
		DrumRadiusMM:  cfg.Hardware.DrumRadiusMM,
		PulsesPerRev:  cfg.Hardware.PulsesPerRev,
		MotorReversed: cfg.Hardware.MotorReversed,

		HomingRPM:       cfg.Homing.RPM,
		HomingTorquePct: cfg.Homing.TorquePct,

		TorqueSafetyPct: cfg.Safety.TorquePct,

		HoldTensionPct: cfg.Hold.TorquePct,
		HoldTensionRPM: cfg.Hold.RPM,

		AccelMmPerSec2: cfg.Move.AccelMmPerSec2,

		ApproachRPM:       cfg.Move.ApproachRPM,
		ApproachFactor:    cfg.Move.ApproachFactor,
		MinApproachPulses: int64(cfg.Move.MinApproachPulses),
		TolerancePulses:   int64(cfg.Move.TolerancePulses),
		PollInterval:      time.Duration(cfg.Move.PollMs) * time.Millisecond,
		StopSettle:        time.Duration(cfg.Move.StopSettleMs) * time.Millisecond,
		DisableWait:       time.Duration(cfg.Move.DisableWaitMs) * time.Millisecond,
		ApproachSwitch:    time.Duration(cfg.Move.ApproachSwitchMs) * time.Millisecond,

		LineTickDT:     time.Duration(cfg.Line.TickMs) * time.Millisecond,
		LineCorrGain:   cfg.Line.CorrectionGain,
		LineFaultEvery: cfg.Line.FaultCheckEvery,
		LineSettleTol:  int32(cfg.Line.SettleTolPulses),
		LineSettleLim:  time.Duration(cfg.Line.SettleTimeoutS*1000) * time.Millisecond,
	}
}
