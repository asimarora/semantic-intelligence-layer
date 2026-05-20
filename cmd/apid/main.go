package main

import (
        "context"
        "fmt"
        "net/http"
        "os"
        "os/signal"
        "syscall"

        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
        sillogging "github.com/asimarora/semantic-intelligence-layer/internal/platform/logging"
        silapi "github.com/asimarora/semantic-intelligence-layer/internal/transport/api"
)

func main() {
        cfg, err := silconfig.Load()
        if err != nil {
                fmt.Fprintf(os.Stderr, "apid bootstrap failed: %v\n", err)
                os.Exit(1)
        }

        logRuntime, err := sillogging.New(cfg.App, cfg.Logging, "apid", os.Stderr)
        if err != nil {
                fmt.Fprintf(os.Stderr, "apid logger failed: %v\n", err)
                os.Exit(1)
        }
        logger := logRuntime.Logger

        server, err := silapi.NewServer(cfg, logger)
        if err != nil {
                fmt.Fprintf(os.Stderr, "apid server failed: %v\n", err)
                os.Exit(1)
        }

        ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
        defer stop()

        go func() {
                <-ctx.Done()
                shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.API.WriteTimeout)
                defer cancel()
                if err := server.Shutdown(shutdownCtx); err != nil {
                        logger.Error("server shutdown failed", "error", err)
                }
        }()

        logger.Info("apid listening", "addr", server.Addr)
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                _ = logRuntime.Close()
                fmt.Fprintf(os.Stderr, "apid serve failed: %v\n", err)
                os.Exit(1)
        }
        if err := logRuntime.Close(); err != nil {
                fmt.Fprintf(os.Stderr, "apid logger shutdown failed: %v\n", err)
                os.Exit(1)
        }
}
