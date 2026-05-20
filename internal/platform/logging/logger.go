package logging

import (
        "context"
        "errors"
        "fmt"
        "io"
        "log/slog"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "time"
        "unicode"

        "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
)

type Runtime struct {
        Logger       *slog.Logger
        flushTimeout time.Duration
        shutdowns    []func(context.Context) error
        once         sync.Once
        shutdownErr  error
}

func New(app config.AppConfig, cfg config.LoggingConfig, component string, writer io.Writer) (*Runtime, error) {
        level, err := parseLevel(cfg.Level)
        if err != nil {
                return nil, err
        }

        sink, fileCloser, err := buildSink(cfg, component, writer)
        if err != nil {
                return nil, err
        }

        options := &slog.HandlerOptions{
                AddSource: cfg.AddSource,
                Level:     level,
        }

        handler, err := newHandler(cfg.Format, sink, options)
        if err != nil {
                if fileCloser != nil {
                        _ = fileCloser.Close()
                }
                return nil, err
        }
        handler = handler.WithAttrs(baseAttrs(app, component))

        runtime := &Runtime{
                flushTimeout: cfg.FlushTimeout,
        }

        if fileCloser != nil {
                runtime.shutdowns = append(runtime.shutdowns, closeWriter(fileCloser))
        }

        if cfg.AsyncEnabled {
                asyncHandler := newAsyncHandler(handler, cfg.AsyncQueueSize)
                handler = asyncHandler
                runtime.shutdowns = append(runtime.shutdowns, asyncHandler.Shutdown)
        }

        runtime.Logger = slog.New(handler)
        return runtime, nil
}

func (r *Runtime) Close() error {
        if r == nil {
                return nil
        }

        ctx, cancel := context.WithTimeout(context.Background(), r.timeout())
        defer cancel()
        return r.Shutdown(ctx)
}

func (r *Runtime) Shutdown(ctx context.Context) error {
        if r == nil {
                return nil
        }
        if ctx == nil {
                ctx = context.Background()
        }

        r.once.Do(func() {
                var errs []error
                for i := len(r.shutdowns) - 1; i >= 0; i-- {
                        if err := r.shutdowns[i](ctx); err != nil {
                                errs = append(errs, err)
                        }
                }
                r.shutdownErr = errors.Join(errs...)
        })
        return r.shutdownErr
}

func (r *Runtime) timeout() time.Duration {
        if r.flushTimeout > 0 {
                return r.flushTimeout
        }
        return 5 * time.Second
}

func newHandler(format string, writer io.Writer, options *slog.HandlerOptions) (slog.Handler, error) {
        switch strings.ToLower(strings.TrimSpace(format)) {
        case "text":
                return slog.NewTextHandler(writer, options), nil
        case "json":
                return slog.NewJSONHandler(writer, options), nil
        default:
                return nil, fmt.Errorf("unsupported log format %q", format)
        }
}

func buildSink(cfg config.LoggingConfig, component string, writer io.Writer) (io.Writer, io.WriteCloser, error) {
        sinkWriter := writer
        if strings.TrimSpace(cfg.OutputDir) == "" {
                if sinkWriter == nil {
                        sinkWriter = os.Stderr
                }
                return sinkWriter, nil, nil
        }

        if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
                return nil, nil, fmt.Errorf("create logging output dir: %w", err)
        }

        name := sanitizeComponentName(component)
        if name == "" {
                name = "service"
        }
        path := filepath.Join(cfg.OutputDir, name+logFileExtension(cfg.Format))
        file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
        if err != nil {
                return nil, nil, fmt.Errorf("open log file %q: %w", path, err)
        }

        if sinkWriter == nil {
                return file, file, nil
        }
        return io.MultiWriter(sinkWriter, file), file, nil
}

func baseAttrs(app config.AppConfig, component string) []slog.Attr {
        attrs := []slog.Attr{
                slog.String("service", app.Name),
                slog.String("env", app.Env),
        }
        if component != "" {
                attrs = append(attrs, slog.String("component", component))
        }
        return attrs
}

func sanitizeComponentName(component string) string {
        component = strings.TrimSpace(component)
        if component == "" {
                return ""
        }

        var builder strings.Builder
        for _, r := range component {
                switch {
                case unicode.IsLetter(r), unicode.IsDigit(r):
                        builder.WriteRune(unicode.ToLower(r))
                case r == '-', r == '_':
                        builder.WriteRune(r)
                default:
                        builder.WriteRune('-')
                }
        }
        return strings.Trim(builder.String(), "-")
}

func closeWriter(writer io.WriteCloser) func(context.Context) error {
        return func(context.Context) error {
                return writer.Close()
        }
}

func logFileExtension(format string) string {
        if strings.EqualFold(strings.TrimSpace(format), "json") {
                return ".jsonl"
        }
        return ".log"
}

func parseLevel(value string) (slog.Level, error) {
        switch strings.ToLower(strings.TrimSpace(value)) {
        case "debug":
                return slog.LevelDebug, nil
        case "info":
                return slog.LevelInfo, nil
        case "warn":
                return slog.LevelWarn, nil
        case "error":
                return slog.LevelError, nil
        default:
                return slog.LevelInfo, fmt.Errorf("unsupported log level %q", value)
        }
}
