package main

import (
        "bytes"
        "context"
        "encoding/json"
        "errors"
        "flag"
        "fmt"
        "io"
        "io/fs"
        "log/slog"
        "os"
        "os/signal"
        "path/filepath"
        "sort"
        "strings"
        "sync"
        "syscall"

        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
        sillogging "github.com/asimarora/semantic-intelligence-layer/internal/platform/logging"
)

type backfillConfig struct {
        InputDir   string
        OutputPath string
        Source     string
        Workers    int
}

type backfillRecord struct {
        Sequence int             `json:"sequence"`
        Source   string          `json:"source"`
        Path     string          `json:"path"`
        Payload  json.RawMessage `json:"payload"`
}

type job struct {
        Index int
        Path  string
}

type result struct {
        Index  int
        Record backfillRecord
        Err    error
}

func main() {
        ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
        defer stop()

        if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
                fmt.Fprintf(os.Stderr, "backfill failed: %v\n", err)
                os.Exit(1)
        }
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
        appConfig, err := silconfig.Load()
        if err != nil {
                return err
        }

        cfg, err := parseBackfillConfig(args, stderr, appConfig)
        if err != nil {
                if errors.Is(err, flag.ErrHelp) {
                        return nil
                }
                return err
        }

        logRuntime, err := sillogging.New(appConfig.App, appConfig.Logging, "backfill", stderr)
        if err != nil {
                return err
        }

        err = runBackfill(ctx, logRuntime.Logger.With("source", cfg.Source), stdout, cfg)
        if closeErr := logRuntime.Close(); closeErr != nil && err == nil {
                err = closeErr
        }
        return err
}

func parseBackfillConfig(args []string, stderr io.Writer, cfg *silconfig.Config) (backfillConfig, error) {
        commandConfig := backfillConfig{
                InputDir:   cfg.Ingest.InputPath,
                OutputPath: "-",
                Source:     cfg.Ingest.SourceType,
                Workers:    cfg.Pipeline.Workers,
        }

        flags := flag.NewFlagSet("backfill", flag.ContinueOnError)
        flags.SetOutput(stderr)
        flags.StringVar(&commandConfig.InputDir, "input", commandConfig.InputDir, "directory containing JSON fixture files")
        flags.StringVar(&commandConfig.OutputPath, "output", commandConfig.OutputPath, "output path for JSONL ('-' for stdout)")
        flags.StringVar(&commandConfig.Source, "source", commandConfig.Source, "source label to stamp on emitted records")
        flags.IntVar(&commandConfig.Workers, "workers", commandConfig.Workers, "number of concurrent file readers")

        if err := flags.Parse(args); err != nil {
                return backfillConfig{}, err
        }

        if flags.NArg() > 0 {
                return backfillConfig{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), ", "))
        }
        if strings.TrimSpace(commandConfig.InputDir) == "" {
                return backfillConfig{}, fmt.Errorf("input directory is required")
        }
        if strings.TrimSpace(commandConfig.OutputPath) == "" {
                return backfillConfig{}, fmt.Errorf("output path is required")
        }
        if strings.TrimSpace(commandConfig.Source) == "" {
                return backfillConfig{}, fmt.Errorf("source is required")
        }
        if commandConfig.Workers <= 0 {
                return backfillConfig{}, fmt.Errorf("workers must be positive, got %d", commandConfig.Workers)
        }

        return commandConfig, nil
}

func runBackfill(ctx context.Context, logger *slog.Logger, stdout io.Writer, cfg backfillConfig) error {
        files, err := listJSONFiles(cfg.InputDir)
        if err != nil {
                return err
        }
        if len(files) == 0 {
                return fmt.Errorf("no JSON fixture files found under %s", cfg.InputDir)
        }

        writer, closeWriter, err := openOutput(cfg.OutputPath, stdout)
        if err != nil {
                return err
        }
        defer closeWriter()

        logger.Info(
                "starting backfill",
                "input", cfg.InputDir,
                "files", len(files),
                "workers", cfg.Workers,
                "output", cfg.OutputPath,
        )

        ctx, cancel := context.WithCancel(ctx)
        defer cancel()

        jobs := make(chan job)
        results := make(chan result, cfg.Workers)

        var workerWG sync.WaitGroup
        for workerID := 0; workerID < cfg.Workers; workerID++ {
                workerWG.Add(1)
                go func() {
                        defer workerWG.Done()

                        for {
                                select {
                                case <-ctx.Done():
                                   return
                                case job, ok := <-jobs:
                                   if !ok {
                                        return
                                   }

                                   record, err := processFile(cfg, job)
                                   select {
                                   case <-ctx.Done():
                                        return
                                   case results <- result{Index: job.Index, Record: record, Err: err}:
                                   }
                                }
                        }
                }()
        }

        go func() {
                defer close(jobs)
                for index, path := range files {
                        select {
                        case <-ctx.Done():
                                return
                        case jobs <- job{Index: index, Path: path}:
                        }
                }
        }()

        go func() {
                workerWG.Wait()
                close(results)
        }()

        encoder := json.NewEncoder(writer)
        encoder.SetEscapeHTML(false)

        next := 0
        pending := make(map[int]backfillRecord, cfg.Workers)

        for result := range results {
                if result.Err != nil {
                        cancel()
                        return result.Err
                }

                pending[result.Index] = result.Record
                for {
                        record, ok := pending[next]
                        if !ok {
                                break
                        }

                        if err := encoder.Encode(record); err != nil {
                                cancel()
                                return fmt.Errorf("write output: %w", err)
                        }

                        delete(pending, next)
                        next++
                }
        }

        if next != len(files) {
                return fmt.Errorf("incomplete backfill: wrote %d of %d records", next, len(files))
        }

        logger.Info("backfill completed", "records", next)
        return nil
}

func processFile(cfg backfillConfig, job job) (backfillRecord, error) {
        path := filepath.Join(cfg.InputDir, job.Path)

        data, err := os.ReadFile(path)
        if err != nil {
                return backfillRecord{}, fmt.Errorf("read %s: %w", job.Path, err)
        }

        payload := bytes.TrimSpace(data)
        if len(payload) == 0 {
                return backfillRecord{}, fmt.Errorf("fixture %s is empty", job.Path)
        }
        if !json.Valid(payload) {
                return backfillRecord{}, fmt.Errorf("fixture %s does not contain valid JSON", job.Path)
        }

        return backfillRecord{
                Sequence: job.Index,
                Source:   cfg.Source,
                Path:     job.Path,
                Payload:  json.RawMessage(payload),
        }, nil
}

func listJSONFiles(root string) ([]string, error) {
        info, err := os.Stat(root)
        if err != nil {
                return nil, fmt.Errorf("stat input directory %s: %w", root, err)
        }
        if !info.IsDir() {
                return nil, fmt.Errorf("input path %s is not a directory", root)
        }

        var files []string
        err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
                if walkErr != nil {
                        return walkErr
                }
                if entry.IsDir() {
                        return nil
                }
                if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
                        return nil
                }

                relPath, err := filepath.Rel(root, path)
                if err != nil {
                        return fmt.Errorf("compute relative path for %s: %w", path, err)
                }

                files = append(files, filepath.ToSlash(relPath))
                return nil
        })
        if err != nil {
                return nil, fmt.Errorf("walk input directory %s: %w", root, err)
        }

        sort.Strings(files)
        return files, nil
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
