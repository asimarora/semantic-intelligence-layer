package api

import (
        "fmt"
        "log/slog"
        "net/http"

        channelcommon "github.com/asimarora/semantic-intelligence-layer/internal/channels/common"
        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
        sessionquery "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
        retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
        sessionmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
        "github.com/asimarora/semantic-intelligence-layer/internal/transport/webhook"
)

func NewServer(cfg *silconfig.Config, logger *slog.Logger) (*http.Server, error) {
        if cfg == nil {
                return nil, fmt.Errorf("config is required")
        }

        registry, err := channelcommon.LoadConfigs(cfg.Channels.ConfigDir, cfg.Channels.Enabled, cfg.Channels.WebhookBasePath)
        if err != nil {
                return nil, err
        }

        mux := http.NewServeMux()
        mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
                w.Header().Set("Content-Type", "application/json")
                _, _ = w.Write([]byte(`{"status":"ok"}`))
        })
        mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
                w.Header().Set("Content-Type", "application/json")
                _, _ = w.Write([]byte(`{"status":"ready"}`))
        })

        sessionStore, sessionInitErr := sessionmetadata.NewStore(cfg.Storage.Metadata)
        var sessionService *sessionquery.SessionService
        if sessionInitErr == nil {
                sessionService, sessionInitErr = sessionquery.NewSessionService(sessionStore)
        }
        registerSessionRoutes(mux, logger, sessionService, sessionInitErr)

        retrievalStore, retrievalInitErr := retrievalmetadata.NewStore(cfg.Storage.Metadata)
        var retrievalService *sessionquery.RetrievalService
        if retrievalInitErr == nil {
                retrievalService, retrievalInitErr = sessionquery.NewRetrievalService(retrievalStore)
        }
        registerRetrievalRoutes(mux, logger, retrievalService, retrievalInitErr)

        if err := webhook.Register(mux, logger, registry, int64(cfg.Channels.RequestBodyLimitBytes)); err != nil {
                return nil, err
        }

        return &http.Server{
                Addr:         cfg.API.ListenAddr,
                Handler:      mux,
                ReadTimeout:  cfg.API.ReadTimeout,
                WriteTimeout: cfg.API.WriteTimeout,
                IdleTimeout:  cfg.API.IdleTimeout,
        }, nil
}
