package main

import (
        "context"
        "fmt"
        "os"
        "os/signal"
        "strings"
        "syscall"

        "github.com/asimarora/semantic-intelligence-layer/internal/adapters/radius"
        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
        sillogging "github.com/asimarora/semantic-intelligence-layer/internal/platform/logging"
        "github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
        "github.com/asimarora/semantic-intelligence-layer/internal/services/ingest"
        rawstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/raw"
)

func main() {
        ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
        defer stop()

        if err := run(ctx); err != nil {
                fmt.Fprintf(os.Stderr, "ingestd failed: %v\n", err)
                os.Exit(1)
        }
}

func run(ctx context.Context) error {
        cfg, err := silconfig.Load()
        if err != nil {
                return err
        }
        if !radius.SupportsSourceType(cfg.Ingest.SourceType) {
                return fmt.Errorf("unsupported ingest source type %q", cfg.Ingest.SourceType)
        }
        if !sourceEnabled(cfg.Sources.Enabled, radius.Kind) {
                return fmt.Errorf("radius source is not enabled in sources.enabled")
        }

        logger, err := sillogging.New(cfg.App, cfg.Logging, "ingestd", os.Stderr)
        if err != nil {
                return err
        }

        sourceConfig, err := radius.LoadConfig(cfg.Sources.ConfigDir)
        if err != nil {
                return err
        }
        sourceConfig.InputPath = sourceConfig.EffectiveInputPath(cfg.Ingest.InputPath)

        store, err := rawstore.NewStore(cfg.Storage.Raw)
        if err != nil {
                return err
        }

        var publisher messaging.Publisher
        var streamClient *messaging.Client
        if !strings.EqualFold(strings.TrimSpace(cfg.Stream.Backend), "disabled") {
                streamClient, err = messaging.Connect(cfg.Stream)
                if err != nil {
                        return err
                }
                defer streamClient.Close()

                if err := streamClient.EnsureConfiguredStream(ctx, cfg.Stream); err != nil {
                        return err
                }
                publisher = streamClient
        }

        service := ingest.Service{
                Logger:    logger,
                Source:    sourceConfig,
                Store:     store,
                Publisher: publisher,
        }
        _, err = service.Run(ctx, sourceConfig.InputPath)
        return err
}

func sourceEnabled(enabled []string, source string) bool {
        for _, value := range enabled {
                if strings.EqualFold(strings.TrimSpace(value), source) {
                        return true
                }
        }
        return false
}
