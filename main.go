package main

import (
	"context"
	"os/signal"
	"syscall"

	_ "go.uber.org/automaxprocs"

	"github.com/crtsh/ctsubmit/logger"
	"github.com/crtsh/ctsubmit/monitor"
	"github.com/crtsh/ctsubmit/server"
)

func main() {
	// Configure graceful shutdown capabilities.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	defer logger.Logger.Info("Shutting down")
	defer logger.ShutdownWG.Wait()

	// Start the various goroutines.
	logger.ShutdownWG.Add(2)
	monitor.FetchEndpointUptimes() // Initial fetch before starting the periodic fetcher.
	go monitor.UptimeFetcher(ctx)
	monitor.FetchAllSTHs() // Initial fetch before starting the periodic fetcher.
	go monitor.STHMonitor(ctx)

	// Start the HTTP servers (Web and Monitoring).
	server.Run()
	defer server.Shutdown()

	// Wait to be interrupted.
	<-ctx.Done()

	// Ensure all log messages are flushed before we exit.
	logger.Logger.Sync()
}
