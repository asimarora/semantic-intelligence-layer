package api

import (
	"fmt"
	"log/slog"
	"net/http"

	agentharness "github.com/asimarora/semantic-intelligence-layer/internal/agents/harness"
	channelcommon "github.com/asimarora/semantic-intelligence-layer/internal/channels/common"
	silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
	sessionquery "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
	accessmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/access"
	agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
	retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
	sessionmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
	a2atransport "github.com/asimarora/semantic-intelligence-layer/internal/transport/a2a"
	chattransport "github.com/asimarora/semantic-intelligence-layer/internal/transport/chat"
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

	accessStore, accessInitErr := accessmetadata.NewStore(cfg.Storage.Metadata)
	var accessService *sessionquery.AccessService
	if accessInitErr == nil {
		accessService, accessInitErr = sessionquery.NewAccessService(accessStore)
	}
	registerAccessRoutes(mux, logger, accessService, accessInitErr)

	retrievalStore, retrievalInitErr := retrievalmetadata.NewStore(cfg.Storage.Metadata)
	var retrievalService *sessionquery.RetrievalService
	if retrievalInitErr == nil {
		retrievalService, retrievalInitErr = sessionquery.NewRetrievalService(retrievalStore)
	}
	registerRetrievalRoutes(mux, logger, retrievalService, retrievalInitErr)

	agentStore, agentInitErr := agentmetadata.NewStore(cfg.Storage.Metadata)
	var agentService *agentharness.Service
	if agentInitErr == nil {
		switch {
		case retrievalInitErr != nil:
			agentInitErr = retrievalInitErr
		default:
			agentService, agentInitErr = agentharness.NewService(agentharness.Dependencies{
				Logger:    logger,
				Runs:      agentStore,
				Retrieval: retrievalService,
				Access:    accessService,
				Sessions:  sessionService,
			})
		}
	}
	registerAgentRoutes(mux, logger, agentService, agentInitErr)

	var chatHandler *chattransport.Handler
	chatInitErr := agentInitErr
	if chatInitErr == nil {
		chatHandler, chatInitErr = chattransport.NewHandler(chattransport.Dependencies{
			Logger:  logger,
			Harness: agentService,
		})
	}
	registerChatRoutes(mux, logger, chatHandler, chatInitErr)

	var a2aHandler *a2atransport.Handler
	a2aInitErr := agentInitErr
	if a2aInitErr == nil {
		a2aHandler, a2aInitErr = a2atransport.NewHandler(a2atransport.Dependencies{
			Logger:  logger,
			Harness: agentService,
		})
	}
	registerA2ARoutes(mux, logger, a2aHandler, a2aInitErr)

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
