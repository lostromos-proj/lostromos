package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lostromos-proj/lostromos/config"
	"github.com/lostromos-proj/lostromos/input/constraint"
	"github.com/lostromos-proj/lostromos/metrics"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	level := slog.LevelWarn
	if cfg.Verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	// Set up signal handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startInput := func(name string, run func(context.Context) error) {
		go func() {
			if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				select {
				case errCh <- fmt.Errorf("%s: %w", name, err):
				default:
					fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
				}
			}
		}()
	}

	handler := metrics.NewHandler()
	go handler.Run(ctx)

	addr := fmt.Sprintf("%s:%d", cfg.MetricsHost, cfg.MetricsPort)
	metricsMux := http.NewServeMux()
	metricsMux.Handle(cfg.MetricsPath, handler.PrometheusHandler())
	metricsServer := &http.Server{
		Addr:    addr,
		Handler: metricsMux,
	}

	go func() {
		err := metricsServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- fmt.Errorf("metrics server failed: %w", err):
			default:
				fmt.Fprintf(os.Stderr, "metrics server failed: %v\n", err)
			}
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = metricsServer.Shutdown(shutdownCtx)
	}()

	// Start constraint input if not disabled
	if !cfg.DisableConstraintInput {
		dynamicClient, err := config.NewDynamicClient(cfg.Kubeconfig)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to create dynamic client:", err)
			os.Exit(1)
		}
		startInput("constraint input", constraint.NewRunner(dynamicClient, handler.Updates()).Run)
	}

	// Wait for signal or error
	select {
	case <-stop:
		cancel()
	case err := <-errCh:
		cancel()
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
