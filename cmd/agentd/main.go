package main

import (
        "context"
        "errors"
        "flag"
        "fmt"
        "io"
        "os"
        "os/signal"
        "strings"
        "syscall"
        "time"

        agentharness "github.com/asimarora/semantic-intelligence-layer/internal/agents/harness"
        agentruntime "github.com/asimarora/semantic-intelligence-layer/internal/agents/runtime"
        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
        sillogging "github.com/asimarora/semantic-intelligence-layer/internal/platform/logging"
        queryservice "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
        agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
        retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
        sessionmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
)

type commandConfig struct {
        Tenants               []string
        PollInterval          time.Duration
        Window                time.Duration
        MinEvents             int
        ShortSessionThreshold time.Duration
        MaxRunsPerCycle       int
        Once                  bool
}

func main() {
        ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
        defer stop()

        if err := run(ctx, os.Args[1:], os.Stderr); err != nil {
                fmt.Fprintf(os.Stderr, "agentd failed: %v\n", err)
                os.Exit(1)
        }
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
        cfg, err := silconfig.Load()
        if err != nil {
                return err
        }

        commandConfig, err := parseCommandConfig(args, stderr)
        if err != nil {
                if err == flag.ErrHelp {
                        return nil
                }
                return err
        }

        logRuntime, err := sillogging.New(cfg.App, cfg.Logging, "agentd", stderr)
        if err != nil {
                return err
        }

        sessionStore, err := sessionmetadata.NewStore(cfg.Storage.Metadata)
        if err != nil {
                return closeRuntimeWithError(logRuntime, err)
        }
        retrievalStore, err := retrievalmetadata.NewStore(cfg.Storage.Metadata)
        if err != nil {
                return closeRuntimeWithError(logRuntime, err)
        }
        runStore, err := agentmetadata.NewStore(cfg.Storage.Metadata)
        if err != nil {
                return closeRuntimeWithError(logRuntime, err)
        }
        watcherStateStore, err := agentmetadata.NewWatcherStateStore(cfg.Storage.Metadata)
        if err != nil {
                return closeRuntimeWithError(logRuntime, err)
        }

        sessionService, err := queryservice.NewSessionService(sessionStore)
        if err != nil {
                return closeRuntimeWithError(logRuntime, err)
        }
        retrievalService, err := queryservice.NewRetrievalService(retrievalStore)
        if err != nil {
                return closeRuntimeWithError(logRuntime, err)
        }
        harnessService, err := agentharness.NewService(agentharness.Dependencies{
                Logger:    logRuntime.Logger,
                Runs:      runStore,
                Retrieval: retrievalService,
                Sessions:  sessionService,
        })
        if err != nil {
                return closeRuntimeWithError(logRuntime, err)
        }

        runtimeService, err := agentruntime.NewService(agentruntime.Config{
                Tenants:               commandConfig.Tenants,
                PollInterval:          commandConfig.PollInterval,
                Window:                commandConfig.Window,
                MinEvents:             commandConfig.MinEvents,
                ShortSessionThreshold: commandConfig.ShortSessionThreshold,
                MaxRunsPerCycle:       commandConfig.MaxRunsPerCycle,
        }, agentruntime.Dependencies{
                Logger:       logRuntime.Logger,
                Harness:      harnessService,
                Sessions:     sessionService,
                WatcherState: watcherStateStore,
        })
        if err != nil {
                return closeRuntimeWithError(logRuntime, err)
        }

        if commandConfig.Once {
                result, err := runtimeService.RunCycle(ctx, time.Now().UTC())
                logRuntime.Logger.Info("agent watcher one-shot cycle completed", "candidates", result.Candidates, "triggered", result.Triggered, "skipped", result.Skipped)
                if closeErr := logRuntime.Close(); closeErr != nil && err == nil {
                        err = closeErr
                }
                return err
        }

        err = runtimeService.Run(ctx)
        if errors.Is(err, context.Canceled) {
                err = nil
        }
        if closeErr := logRuntime.Close(); closeErr != nil && err == nil {
                err = closeErr
        }
        return err
}

func parseCommandConfig(args []string, stderr io.Writer) (commandConfig, error) {
        defaultTenants := strings.Join(defaultTenantList(), ",")
        config := commandConfig{
                PollInterval:          30 * time.Second,
                Window:                15 * time.Minute,
                MinEvents:             3,
                ShortSessionThreshold: 5 * time.Minute,
                MaxRunsPerCycle:       5,
        }

        var tenantValues string
        flags := flag.NewFlagSet("agentd", flag.ContinueOnError)
        flags.SetOutput(stderr)
        flags.StringVar(&tenantValues, "tenants", defaultTenants, "comma-separated tenant ids to watch")
        flags.DurationVar(&config.PollInterval, "poll-interval", config.PollInterval, "poll interval between daemon cycles")
        flags.DurationVar(&config.Window, "window", config.Window, "lookback window for repeated disconnect detection")
        flags.IntVar(&config.MinEvents, "min-events", config.MinEvents, "minimum qualifying short-session stop events before triggering an investigation")
        flags.DurationVar(&config.ShortSessionThreshold, "short-session-threshold", config.ShortSessionThreshold, "maximum session duration considered a short session")
        flags.IntVar(&config.MaxRunsPerCycle, "max-runs-per-cycle", config.MaxRunsPerCycle, "maximum investigations to trigger per cycle per tenant")
        flags.BoolVar(&config.Once, "once", false, "run a single watcher cycle and exit")
        if err := flags.Parse(args); err != nil {
                return commandConfig{}, err
        }
        if flags.NArg() > 0 {
                return commandConfig{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), ", "))
        }

        config.Tenants = splitList(tenantValues)
        return config, nil
}

func defaultTenantList() []string {
        if values := splitList(os.Getenv("SIL_AGENT_TENANTS")); len(values) > 0 {
                return values
        }
        return []string{"default"}
}

func splitList(value string) []string {
        parts := strings.Split(value, ",")
        values := make([]string, 0, len(parts))
        seen := make(map[string]struct{})
        for _, part := range parts {
                part = strings.TrimSpace(part)
                if part == "" {
                        continue
                }
                if _, ok := seen[part]; ok {
                        continue
                }
                seen[part] = struct{}{}
                values = append(values, part)
        }
        return values
}

func closeRuntimeWithError(runtime *sillogging.Runtime, runErr error) error {
        if runtime == nil {
                return runErr
        }
        if closeErr := runtime.Close(); closeErr != nil {
                return errors.Join(runErr, closeErr)
        }
        return runErr
}
