package logging

import (
        "fmt"
        "io"
        "log/slog"
        "os"
        "strings"

        "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
)

func New(app config.AppConfig, cfg config.LoggingConfig, component string, writer io.Writer) (*slog.Logger, error) {
        level, err := parseLevel(cfg.Level)
        if err != nil {
                return nil, err
        }

        if writer == nil {
                writer = os.Stderr
        }

        options := &slog.HandlerOptions{
                AddSource: cfg.AddSource,
                Level:     level,
        }

        var handler slog.Handler
        switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
        case "text":
                handler = slog.NewTextHandler(writer, options)
        case "json":
                handler = slog.NewJSONHandler(writer, options)
        default:
                return nil, fmt.Errorf("unsupported log format %q", cfg.Format)
        }

        logger := slog.New(handler).With(
                "service", app.Name,
                "env", app.Env,
        )
        if component != "" {
                logger = logger.With("component", component)
        }

        return logger, nil
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
