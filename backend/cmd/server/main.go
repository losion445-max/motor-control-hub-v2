// Command server runs the robot WebSocket API.
//
// Usage:
//
//	./server -port /dev/ttyUSB0 -addr :8080
//
// Connects to the servo drives, starts broadcasting motor status every 200 ms,
// and serves a WebSocket endpoint at /ws for robot control.
// Ctrl-C triggers a graceful shutdown: the current operation is cancelled and
// all motors are disabled before the process exits.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/losion445-max/motor-control-hub-v2/pkg/api"
	"github.com/losion445-max/motor-control-hub-v2/pkg/robot"
	"github.com/losion445-max/motor-control-hub-v2/pkg/runner"
	"github.com/losion445-max/motor-control-hub-v2/pkg/usecase"
)

func main() {
	serialPort := flag.String("port", "/dev/ttyUSB0", "RS-485 serial port")
	listenAddr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	// ── Robot ──────────────────────────────────────────────────────────────────
	cfg := robot.DefaultConfig
	sys := robot.NewSystem(*serialPort, 19200, cfg)
	if err := sys.Connect(); err != nil {
		log.Fatalf("serial connect: %v", err)
	}
	defer sys.Close()
	log.Printf("connected to drives on %s", *serialPort)

	// ── Orchestrator ───────────────────────────────────────────────────────────
	orch := usecase.New(sys)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Poll all four motors every 200 ms and push to subscribers.
	go orch.RunStatusBroadcast(ctx, 200*time.Millisecond)

	// ── HTTP + WebSocket ───────────────────────────────────────────────────────
	wsHandler := api.NewHandler(orch, runner.DefaultOpts)
	srv := api.NewServer(*listenAddr, wsHandler)

	go func() {
		log.Printf("listening on %s  (WebSocket → /ws  health → /health)", *listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down…")

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := api.Shutdown(shutCtx, srv); err != nil {
		log.Printf("shutdown: %v", err)
	}
	if err := sys.EmergencyStop(); err != nil {
		log.Printf("emergency stop: %v", err)
	}
	log.Println("done")
}
