package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/asimarora/semantic-intelligence-layer/internal/adapters/access"
	"github.com/asimarora/semantic-intelligence-layer/internal/adapters/radius"
	silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
	sillogging "github.com/asimarora/semantic-intelligence-layer/internal/platform/logging"
	"github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
	accessnormalize "github.com/asimarora/semantic-intelligence-layer/internal/services/accessnormalize"
	"github.com/asimarora/semantic-intelligence-layer/internal/services/normalize"
	accessmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/access"
	sessionmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
	rawstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/raw"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "normalizerd failed: %v\n", err)
		os.Exit(1)
	}
}

type commandConfig struct {
	OutputPath string
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := silconfig.Load()
	if err != nil {
		return err
	}
	if !radius.SupportsSourceType(cfg.Ingest.SourceType) && !access.SupportsSourceType(cfg.Ingest.SourceType) {
		return fmt.Errorf("unsupported ingest source type %q", cfg.Ingest.SourceType)
	}

	commandConfig, err := parseCommandConfig(args, stderr, cfg)
	if err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	logRuntime, err := sillogging.New(cfg.App, cfg.Logging, "normalizerd", os.Stderr)
	if err != nil {
		return err
	}

	store, err := rawstore.NewStore(cfg.Storage.Raw)
	if err != nil {
		return err
	}

	sessionProjectionStore, err := sessionmetadata.NewStore(cfg.Storage.Metadata)
	if err != nil {
		return err
	}
	accessProjectionStore, err := accessmetadata.NewStore(cfg.Storage.Metadata)
	if err != nil {
		return err
	}

	writer, closeWriter, err := openOutput(commandConfig.OutputPath, stdout)
	if err != nil {
		return err
	}
	defer closeWriter()

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

	switch {
	case radius.SupportsSourceType(cfg.Ingest.SourceType):
		service := normalize.Service{
			Logger:          logRuntime.Logger,
			Store:           store,
			ProjectionStore: sessionProjectionStore,
			Publisher:       publisher,
			UnifiedSubject:  cfg.Stream.UnifiedSubject,
		}
		_, err = service.Run(ctx, writer)
	case access.SupportsSourceType(cfg.Ingest.SourceType):
		service := accessnormalize.Service{
			Logger:          logRuntime.Logger,
			Store:           store,
			ProjectionStore: accessProjectionStore,
			Publisher:       publisher,
			UnifiedSubject:  cfg.Stream.UnifiedSubject,
		}
		_, err = service.Run(ctx, writer)
	}
	if closeErr := logRuntime.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func parseCommandConfig(args []string, stderr io.Writer, cfg *silconfig.Config) (commandConfig, error) {
	commandConfig := commandConfig{
		OutputPath: defaultOutputPath(cfg.Storage.Raw.Path, cfg.Ingest.SourceType),
	}

	flags := flag.NewFlagSet("normalizerd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&commandConfig.OutputPath, "output", commandConfig.OutputPath, "output path for normalized JSONL ('-' for stdout)")
	if err := flags.Parse(args); err != nil {
		return commandConfig, err
	}
	if flags.NArg() > 0 {
		return commandConfig, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), ", "))
	}
	if strings.TrimSpace(commandConfig.OutputPath) == "" {
		return commandConfig, fmt.Errorf("output path is required")
	}
	return commandConfig, nil
}

func defaultOutputPath(rawPath, sourceType string) string {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "-"
	}
	fileName := "network-session-events.jsonl"
	if access.SupportsSourceType(sourceType) {
		fileName = "network-access-events.jsonl"
	}
	return filepath.Join(filepath.Dir(rawPath), "unified", fileName)
}

func openOutput(path string, stdout io.Writer) (io.Writer, func() error, error) {
	if path == "-" {
		return stdout, func() error { return nil }, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create output directory for %s: %w", path, err)
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create output file %s: %w", path, err)
	}

	return file, file.Close, nil
}
