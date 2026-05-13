package main

import (
        "fmt"
        "os"

        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
        sillogging "github.com/asimarora/semantic-intelligence-layer/internal/platform/logging"
)

func main() {
        cfg, err := silconfig.Load()
        if err != nil {
                fmt.Fprintf(os.Stderr, "modeld bootstrap failed: %v\n", err)
                os.Exit(1)
        }

        logger, err := sillogging.New(cfg.App, cfg.Logging, "modeld", os.Stderr)
        if err != nil {
                fmt.Fprintf(os.Stderr, "modeld logger failed: %v\n", err)
                os.Exit(1)
        }

        logger.Info("service scaffold ready")
}
