package main

import (
        "context"
        "errors"
        "flag"
        "fmt"
        "io"
        "os"
        "os/signal"
        "path/filepath"
        "strings"
        "syscall"

        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
        sillogging "github.com/asimarora/semantic-intelligence-layer/internal/platform/logging"
        "github.com/asimarora/semantic-intelligence-layer/internal/services/index"
        retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
)

type commandConfig struct {
        InputPath string
}

func main() {
        ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
        defer stop()

        if err := run(ctx, os.Args[1:], os.Stdin, os.Stderr); err != nil {
                fmt.Fprintf(os.Stderr, "indexerd failed: %v\n", err)
                os.Exit(1)
        }
}

func run(ctx context.Context, args []string, stdin io.Reader, stderr io.Writer) error {
        cfg, err := silconfig.Load()
        if err != nil {
                return err
        }

        commandConfig, err := parseCommandConfig(args, stderr, cfg)
        if err != nil {
                if err == flag.ErrHelp {
                        return nil
                }
                return err
        }

        logRuntime, err := sillogging.New(cfg.App, cfg.Logging, "indexerd", stderr)
        if err != nil {
                return err
        }

        store, err := retrievalmetadata.NewStore(cfg.Storage.Metadata)
        if err != nil {
                closeErr := logRuntime.Close()
                if closeErr != nil {
                        return errors.Join(err, closeErr)
                }
                return err
        }

        reader, closeReader, err := openInput(commandConfig.InputPath, stdin)
        if err != nil {
                closeErr := logRuntime.Close()
                if closeErr != nil {
                        return errors.Join(err, closeErr)
                }
                return err
        }
        defer closeReader()

        service := index.Service{
                Logger: logRuntime.Logger,
                Store:  store,
        }
        _, err = service.Run(ctx, reader)
        if closeErr := logRuntime.Close(); closeErr != nil && err == nil {
                err = closeErr
        }
        return err
}

func parseCommandConfig(args []string, stderr io.Writer, cfg *silconfig.Config) (commandConfig, error) {
        commandConfig := commandConfig{
                InputPath: defaultInputPath(cfg.Storage.Raw.Path),
        }

        flags := flag.NewFlagSet("indexerd", flag.ContinueOnError)
        flags.SetOutput(stderr)
        flags.StringVar(&commandConfig.InputPath, "input", commandConfig.InputPath, "path to normalized network session JSONL ('-' for stdin)")
        if err := flags.Parse(args); err != nil {
                return commandConfig, err
        }
        if flags.NArg() > 0 {
                return commandConfig, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), ", "))
        }
        if strings.TrimSpace(commandConfig.InputPath) == "" {
                return commandConfig, fmt.Errorf("input path is required")
        }

        return commandConfig, nil
}

func defaultInputPath(rawPath string) string {
        rawPath = strings.TrimSpace(rawPath)
        if rawPath == "" {
                return "-"
        }
        return filepath.Join(filepath.Dir(rawPath), "unified", "network-session-events.jsonl")
}

func openInput(path string, stdin io.Reader) (io.Reader, func() error, error) {
        if path == "-" {
                if stdin == nil {
                        return nil, nil, fmt.Errorf("stdin is not available")
                }
                return stdin, func() error { return nil }, nil
        }

        file, err := os.Open(path)
        if err != nil {
                return nil, nil, fmt.Errorf("open input file %s: %w", path, err)
        }
        return file, file.Close, nil
}
