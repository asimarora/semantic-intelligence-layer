package ingest

import (
        "context"
        "fmt"
        "io"
        "io/fs"
        "log/slog"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "time"

        "github.com/asimarora/semantic-intelligence-layer/internal/adapters/radius"
        "github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
        rawstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/raw"
)

type Service struct {
        Logger    *slog.Logger
        Source    radius.Config
        Store     rawstore.Store
        Publisher messaging.Publisher
}

type Result struct {
        Files     int
        Persisted int
        Published int
}

func (service Service) Run(ctx context.Context, inputDir string) (Result, error) {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := service.Source.Validate(); err != nil {
                return Result{}, err
        }
        if service.Store == nil {
                return Result{}, fmt.Errorf("raw store is required")
        }

        inputDir = service.Source.EffectiveInputPath(inputDir)
        if strings.TrimSpace(inputDir) == "" {
                return Result{}, fmt.Errorf("input directory is required")
        }

        files, err := listJSONFiles(inputDir)
        if err != nil {
                return Result{}, err
        }
        if len(files) == 0 {
                return Result{}, fmt.Errorf("no JSON fixture files found under %s", inputDir)
        }

        logger := service.logger().With(
                "source", radius.Source,
                "input", inputDir,
                "files", len(files),
        )
        logger.Info("starting ingest")

        var result Result
        for _, relPath := range files {
                if err := ctx.Err(); err != nil {
                        return result, err
                }

                payload, err := os.ReadFile(filepath.Join(inputDir, relPath))
                if err != nil {
                        return result, fmt.Errorf("read %s: %w", relPath, err)
                }

                adapted, err := radius.Adapt(service.Source, payload, time.Now().UTC())
                if err != nil {
                        return result, fmt.Errorf("adapt %s: %w", relPath, err)
                }
                if err := service.Store.Append(ctx, adapted.Record); err != nil {
                        return result, fmt.Errorf("persist %s: %w", relPath, err)
                }

                result.Files++
                result.Persisted++

                if service.Publisher != nil {
                        if _, err := service.Publisher.PublishEnvelope(ctx, service.Source.Subject, adapted.Envelope); err != nil {
                                return result, fmt.Errorf("publish %s: %w", relPath, err)
                        }
                        result.Published++
                }

                logger.Debug(
                        "ingested radius evidence",
                        "path", relPath,
                        "event_id", adapted.Record.EventID,
                        "source_key", adapted.Record.SourceKey,
                )
        }

        logger.Info(
                "ingest completed",
                "files", result.Files,
                "persisted", result.Persisted,
                "published", result.Published,
        )
        return result, nil
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

func (service Service) logger() *slog.Logger {
        if service.Logger != nil {
                return service.Logger
        }
        return slog.New(slog.NewTextHandler(io.Discard, nil))
}
