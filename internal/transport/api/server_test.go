package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
	retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
	unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
	silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
	accessmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/access"
	retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
	sessionmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
	"github.com/gorilla/websocket"
)

func TestNewServerRegistersHealthAndChannelWebhook(t *testing.T) {
	configDir := t.TempDir()
	writeFile(t, filepath.Join(configDir, "chat.yaml"), `
id: "chat"
kind: "chat"
enabled: true
tenant_id: "tenant-a"
display_name: "Chat"
provider: "web-widget"
webhook_path: "/v1/channels/chat/webhook"
`)

	cfg := &silconfig.Config{
		API: silconfig.APIConfig{
			ListenAddr:   ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
		Channels: silconfig.ChannelsConfig{
			ConfigDir:             configDir,
			Enabled:               []string{"chat"},
			WebhookBasePath:       "/v1/channels",
			RequestBodyLimitBytes: 1 << 20,
		},
	}

	server, err := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	health := httptest.NewRecorder()
	server.Handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", health.Code, http.StatusOK)
	}

	body := strings.NewReader(`{"tenant_id":"tenant-a","conversation_id":"conv-1","message_id":"msg-1","body":"help"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/channels/chat/webhook", body)
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("webhook status = %d, want %d", response.Code, http.StatusAccepted)
	}

	var ack struct {
		Accepted bool   `json:"accepted"`
		Channel  string `json:"channel"`
		EventID  string `json:"event_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &ack); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if !ack.Accepted || ack.Channel != "chat" || ack.EventID == "" {
		t.Fatalf("unexpected ack payload: %+v", ack)
	}
}

func TestNewServerSearchesProjectedSessions(t *testing.T) {
	metadataPath := t.TempDir()
	store, err := sessionmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	sessionTime := uint64(100)
	inputOctets := uint64(12000)
	outputOctets := uint64(54000)
	if err := store.Append(context.Background(), unifiedsessions.Event{
		EventID:            "evt-001",
		SourceEventID:      "raw-001",
		Source:             "ras",
		SourceKey:          "radius:acct:john:sess-001:20260505T140000.000000",
		SchemaVersion:      unifiedsessions.SchemaVersion,
		EventType:          unifiedsessions.EventType,
		TenantID:           "default",
		OccurredAt:         time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
		IngestedAt:         time.Date(2026, 5, 5, 14, 1, 0, 0, time.UTC),
		SubscriberID:       "john",
		SessionID:          "sess-001",
		Status:             unifiedsessions.StatusStop,
		NASIPAddress:       "192.168.1.1",
		ClientIPAddress:    "10.0.0.25",
		SessionTimeSeconds: &sessionTime,
		InputOctets:        &inputOctets,
		OutputOctets:       &outputOctets,
		SemanticText:       "user john disconnected from NAS 192.168.1.1 after a 100 second session with 12000 input octets and 54000 output octets",
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	cfg := &silconfig.Config{
		API: silconfig.APIConfig{
			ListenAddr:   ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
		Storage: silconfig.StorageConfig{
			Metadata: silconfig.StoreConfig{
				Backend: "file",
				Path:    metadataPath,
			},
		},
	}

	server, err := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/sessions?tenant_id=default&subscriber_id=john", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("session lookup status = %d, want %d", response.Code, http.StatusOK)
	}

	var payload struct {
		Count    int                     `json:"count"`
		Sessions []unifiedsessions.Event `json:"sessions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode session lookup response: %v", err)
	}
	if payload.Count != 1 || len(payload.Sessions) != 1 {
		t.Fatalf("unexpected session lookup payload: %+v", payload)
	}
	if payload.Sessions[0].SessionID != "sess-001" {
		t.Fatalf("expected session sess-001, got %q", payload.Sessions[0].SessionID)
	}
}

func TestNewServerSearchesProjectedAccessEvents(t *testing.T) {
	metadataPath := t.TempDir()
	store, err := accessmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if err := store.Append(context.Background(), unifiedaccess.Event{
		EventID:         "evt-access-001",
		SourceEventID:   "raw-access-001",
		Source:          "aaa_auth",
		SourceKey:       "access:auth:john:req-001:20260505T140000.000000",
		SchemaVersion:   unifiedaccess.SchemaVersion,
		EventType:       unifiedaccess.EventType,
		TenantID:        "default",
		OccurredAt:      time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
		IngestedAt:      time.Date(2026, 5, 5, 14, 1, 0, 0, time.UTC),
		SubscriberID:    "john",
		RequestID:       "req-001",
		SessionID:       "sess-001",
		Outcome:         unifiedaccess.OutcomeRejected,
		RejectReason:    "invalid password",
		NASIPAddress:    "192.168.1.1",
		ClientIPAddress: "10.0.0.25",
		SemanticText:    "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	cfg := &silconfig.Config{
		API: silconfig.APIConfig{
			ListenAddr:   ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
		Storage: silconfig.StorageConfig{
			Metadata: silconfig.StoreConfig{
				Backend: "file",
				Path:    metadataPath,
			},
		},
	}

	server, err := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/access-events?tenant_id=default&subscriber_id=john&request_id=req-001&outcome=rejected", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("access lookup status = %d, want %d", response.Code, http.StatusOK)
	}

	var payload struct {
		Count  int                   `json:"count"`
		Events []unifiedaccess.Event `json:"events"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode access lookup response: %v", err)
	}
	if payload.Count != 1 || len(payload.Events) != 1 {
		t.Fatalf("unexpected access lookup payload: %+v", payload)
	}
	if payload.Events[0].RequestID != "req-001" {
		t.Fatalf("expected request req-001, got %q", payload.Events[0].RequestID)
	}
}

func TestNewServerRunsCorrelatedAgentInvestigations(t *testing.T) {
	metadataPath := t.TempDir()
	accessStore, err := accessmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("accessmetadata.NewStore() error = %v", err)
	}
	sessionStore, err := sessionmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("sessionmetadata.NewStore() error = %v", err)
	}
	retrievalStore, err := retrievalmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("retrievalmetadata.NewStore() error = %v", err)
	}

	for _, event := range []unifiedaccess.Event{
		{
			EventID:         "evt-access-001",
			SourceEventID:   "raw-access-001",
			Source:          "aaa_auth",
			SourceKey:       "access:auth:john:req-001:20260505T135500.000000",
			SchemaVersion:   unifiedaccess.SchemaVersion,
			EventType:       unifiedaccess.EventType,
			TenantID:        "default",
			OccurredAt:      time.Date(2026, 5, 5, 13, 55, 0, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 56, 0, 0, time.UTC),
			SubscriberID:    "john",
			RequestID:       "req-001",
			SessionID:       "sess-001",
			Outcome:         unifiedaccess.OutcomeRejected,
			RejectReason:    "invalid password",
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			SemanticText:    "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
		},
		{
			EventID:         "evt-access-002",
			SourceEventID:   "raw-access-002",
			Source:          "aaa_auth",
			SourceKey:       "access:auth:john:req-002:20260505T135800.000000",
			SchemaVersion:   unifiedaccess.SchemaVersion,
			EventType:       unifiedaccess.EventType,
			TenantID:        "default",
			OccurredAt:      time.Date(2026, 5, 5, 13, 58, 0, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 58, 30, 0, time.UTC),
			SubscriberID:    "john",
			RequestID:       "req-002",
			SessionID:       "sess-001",
			Outcome:         unifiedaccess.OutcomeAccepted,
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			SemanticText:    "user john authenticated successfully on NAS 192.168.1.1 from client 10.0.0.25",
		},
	} {
		if err := accessStore.Append(context.Background(), event); err != nil {
			t.Fatalf("accessStore.Append() error = %v", err)
		}
	}

	sessionTime := uint64(100)
	for _, event := range []unifiedsessions.Event{
		{
			EventID:         "evt-session-001",
			SourceEventID:   "raw-session-001",
			Source:          "ras",
			SourceKey:       "radius:acct:john:sess-001:20260505T135820.000000",
			SchemaVersion:   unifiedsessions.SchemaVersion,
			EventType:       unifiedsessions.EventType,
			TenantID:        "default",
			OccurredAt:      time.Date(2026, 5, 5, 13, 58, 20, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 59, 0, 0, time.UTC),
			SubscriberID:    "john",
			SessionID:       "sess-001",
			Status:          unifiedsessions.StatusStart,
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			SemanticText:    "user john started a network session from NAS 192.168.1.1",
		},
		{
			EventID:            "evt-session-002",
			SourceEventID:      "raw-session-002",
			Source:             "ras",
			SourceKey:          "radius:acct:john:sess-001:20260505T140000.000000",
			SchemaVersion:      unifiedsessions.SchemaVersion,
			EventType:          unifiedsessions.EventType,
			TenantID:           "default",
			OccurredAt:         time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
			IngestedAt:         time.Date(2026, 5, 5, 14, 1, 0, 0, time.UTC),
			SubscriberID:       "john",
			SessionID:          "sess-001",
			Status:             unifiedsessions.StatusStop,
			NASIPAddress:       "192.168.1.1",
			ClientIPAddress:    "10.0.0.25",
			SessionTimeSeconds: &sessionTime,
			SemanticText:       "user john disconnected from NAS 192.168.1.1 after a 100 second session",
		},
	} {
		if err := sessionStore.Append(context.Background(), event); err != nil {
			t.Fatalf("sessionStore.Append() error = %v", err)
		}
	}

	document := retrievalcontracts.Document{
		DocumentID:      "evt-access-doc-001",
		SchemaVersion:   retrievalcontracts.SchemaVersion,
		TenantID:        "default",
		Source:          "aaa_auth",
		SourceEventID:   "raw-access-001",
		SourceKey:       "access:auth:john:req-001:20260505T135500.000000",
		EntityType:      retrievalcontracts.EntityTypeNetworkAccess,
		EventType:       unifiedaccess.EventType,
		OccurredAt:      time.Date(2026, 5, 5, 13, 55, 0, 0, time.UTC),
		IngestedAt:      time.Date(2026, 5, 5, 13, 56, 0, 0, time.UTC),
		Title:           "network access rejected for subscriber john on request req-001 on NAS 192.168.1.1",
		Content:         "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
		SubscriberID:    "john",
		SessionID:       "sess-001",
		RequestID:       "req-001",
		Outcome:         "rejected",
		NASIPAddress:    "192.168.1.1",
		ClientIPAddress: "10.0.0.25",
		RejectReason:    "invalid password",
		Terms:           retrievalcontracts.NormalizeTerms("john invalid password then connected", "invalid password", "john", "sess-001"),
	}
	if err := retrievalStore.Append(context.Background(), document); err != nil {
		t.Fatalf("retrievalStore.Append() error = %v", err)
	}

	cfg := &silconfig.Config{
		API: silconfig.APIConfig{
			ListenAddr:   ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
		Storage: silconfig.StorageConfig{
			Metadata: silconfig.StoreConfig{
				Backend: "file",
				Path:    metadataPath,
			},
		},
	}

	server, err := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/agent-runs", strings.NewReader(`{"tenant_id":"default","goal":"Did john fail auth before connecting?","query":"john invalid password then connected","filters":{"subscriber_id":"john"},"max_steps":4}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("agent run status = %d, want %d body=%s", response.Code, http.StatusOK, response.Body.String())
	}

	var payload struct {
		Run struct {
			Status string `json:"Status"`
		} `json:"run"`
		Steps []struct {
			ToolName string `json:"ToolName"`
			Summary  string `json:"Summary"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode agent run response: %v", err)
	}
	if payload.Run.Status != "completed" {
		t.Fatalf("expected completed run status, got %q", payload.Run.Status)
	}
	if len(payload.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(payload.Steps))
	}
	if payload.Steps[1].ToolName != "query.access.search" {
		t.Fatalf("expected access enrichment step, got %q", payload.Steps[1].ToolName)
	}
	if !strings.Contains(payload.Steps[3].Summary, "later accepted") {
		t.Fatalf("expected correlated acceptance in summary, got %q", payload.Steps[3].Summary)
	}
	if !strings.Contains(payload.Steps[3].Summary, "started session sess-001") {
		t.Fatalf("expected correlated session start in summary, got %q", payload.Steps[3].Summary)
	}
	if !strings.HasPrefix(payload.Steps[3].Summary, "Yes.") {
		t.Fatalf("expected direct answer prefix in summary, got %q", payload.Steps[3].Summary)
	}
}

func TestNewServerServesChatAndA2AInterfaces(t *testing.T) {
	metadataPath := t.TempDir()
	accessStore, err := accessmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("accessmetadata.NewStore() error = %v", err)
	}
	sessionStore, err := sessionmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("sessionmetadata.NewStore() error = %v", err)
	}
	retrievalStore, err := retrievalmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("retrievalmetadata.NewStore() error = %v", err)
	}

	for _, event := range []unifiedaccess.Event{
		{
			EventID:         "evt-access-001",
			SourceEventID:   "raw-access-001",
			Source:          "aaa_auth",
			SourceKey:       "access:auth:john:req-001:20260505T135500.000000",
			SchemaVersion:   unifiedaccess.SchemaVersion,
			EventType:       unifiedaccess.EventType,
			TenantID:        "default",
			OccurredAt:      time.Date(2026, 5, 5, 13, 55, 0, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 56, 0, 0, time.UTC),
			SubscriberID:    "john",
			RequestID:       "req-001",
			SessionID:       "sess-001",
			Outcome:         unifiedaccess.OutcomeRejected,
			RejectReason:    "invalid password",
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			SemanticText:    "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
		},
		{
			EventID:         "evt-access-002",
			SourceEventID:   "raw-access-002",
			Source:          "aaa_auth",
			SourceKey:       "access:auth:john:req-002:20260505T135800.000000",
			SchemaVersion:   unifiedaccess.SchemaVersion,
			EventType:       unifiedaccess.EventType,
			TenantID:        "default",
			OccurredAt:      time.Date(2026, 5, 5, 13, 58, 0, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 58, 30, 0, time.UTC),
			SubscriberID:    "john",
			RequestID:       "req-002",
			SessionID:       "sess-001",
			Outcome:         unifiedaccess.OutcomeAccepted,
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			SemanticText:    "user john authenticated successfully on NAS 192.168.1.1 from client 10.0.0.25",
		},
	} {
		if err := accessStore.Append(context.Background(), event); err != nil {
			t.Fatalf("accessStore.Append() error = %v", err)
		}
	}

	sessionTime := uint64(100)
	for _, event := range []unifiedsessions.Event{
		{
			EventID:         "evt-session-001",
			SourceEventID:   "raw-session-001",
			Source:          "ras",
			SourceKey:       "radius:acct:john:sess-001:20260505T135820.000000",
			SchemaVersion:   unifiedsessions.SchemaVersion,
			EventType:       unifiedsessions.EventType,
			TenantID:        "default",
			OccurredAt:      time.Date(2026, 5, 5, 13, 58, 20, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 59, 0, 0, time.UTC),
			SubscriberID:    "john",
			SessionID:       "sess-001",
			Status:          unifiedsessions.StatusStart,
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			SemanticText:    "user john started a network session from NAS 192.168.1.1",
		},
		{
			EventID:            "evt-session-002",
			SourceEventID:      "raw-session-002",
			Source:             "ras",
			SourceKey:          "radius:acct:john:sess-001:20260505T140000.000000",
			SchemaVersion:      unifiedsessions.SchemaVersion,
			EventType:          unifiedsessions.EventType,
			TenantID:           "default",
			OccurredAt:         time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
			IngestedAt:         time.Date(2026, 5, 5, 14, 1, 0, 0, time.UTC),
			SubscriberID:       "john",
			SessionID:          "sess-001",
			Status:             unifiedsessions.StatusStop,
			NASIPAddress:       "192.168.1.1",
			ClientIPAddress:    "10.0.0.25",
			SessionTimeSeconds: &sessionTime,
			SemanticText:       "user john disconnected from NAS 192.168.1.1 after a 100 second session",
		},
	} {
		if err := sessionStore.Append(context.Background(), event); err != nil {
			t.Fatalf("sessionStore.Append() error = %v", err)
		}
	}

	document := retrievalcontracts.Document{
		DocumentID:      "evt-access-doc-001",
		SchemaVersion:   retrievalcontracts.SchemaVersion,
		TenantID:        "default",
		Source:          "aaa_auth",
		SourceEventID:   "raw-access-001",
		SourceKey:       "access:auth:john:req-001:20260505T135500.000000",
		EntityType:      retrievalcontracts.EntityTypeNetworkAccess,
		EventType:       unifiedaccess.EventType,
		OccurredAt:      time.Date(2026, 5, 5, 13, 55, 0, 0, time.UTC),
		IngestedAt:      time.Date(2026, 5, 5, 13, 56, 0, 0, time.UTC),
		Title:           "network access rejected for subscriber john on request req-001 on NAS 192.168.1.1",
		Content:         "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
		SubscriberID:    "john",
		SessionID:       "sess-001",
		RequestID:       "req-001",
		Outcome:         "rejected",
		NASIPAddress:    "192.168.1.1",
		ClientIPAddress: "10.0.0.25",
		RejectReason:    "invalid password",
		Terms:           retrievalcontracts.NormalizeTerms("john invalid password then connected", "invalid password", "john", "sess-001"),
	}
	if err := retrievalStore.Append(context.Background(), document); err != nil {
		t.Fatalf("retrievalStore.Append() error = %v", err)
	}

	cfg := &silconfig.Config{
		API: silconfig.APIConfig{
			ListenAddr:   ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
		Storage: silconfig.StorageConfig{
			Metadata: silconfig.StoreConfig{
				Backend: "file",
				Path:    metadataPath,
			},
		},
	}

	server, err := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	httpServer := httptest.NewServer(server.Handler)
	defer httpServer.Close()

	pageResponse, err := http.Get(httpServer.URL + "/chat")
	if err != nil {
		t.Fatalf("GET /chat error = %v", err)
	}
	defer pageResponse.Body.Close()
	if pageResponse.StatusCode != http.StatusOK {
		t.Fatalf("chat page status = %d, want %d", pageResponse.StatusCode, http.StatusOK)
	}
	pageBody, err := io.ReadAll(pageResponse.Body)
	if err != nil {
		t.Fatalf("read chat page: %v", err)
	}
	if !strings.Contains(string(pageBody), "SIL Chat Demo") {
		t.Fatalf("chat page missing title: %s", pageBody)
	}
	if !strings.Contains(string(pageBody), "Short answer") {
		t.Fatalf("chat page missing short answer summary: %s", pageBody)
	}
	if !strings.Contains(string(pageBody), "Details") {
		t.Fatalf("chat page missing details tab: %s", pageBody)
	}

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/chat/ws"
	connection, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer connection.Close()

	var welcome struct {
		Type string `json:"type"`
	}
	if err := connection.ReadJSON(&welcome); err != nil {
		t.Fatalf("read welcome message: %v", err)
	}
	if welcome.Type != "welcome" {
		t.Fatalf("expected welcome message, got %#v", welcome)
	}

	if err := connection.WriteJSON(map[string]any{
		"type":      "ask",
		"tenant_id": "default",
		"message":   "Did john fail auth before connecting?",
		"filters": map[string]any{
			"subscriber_id": "john",
		},
		"max_steps": 3,
	}); err != nil {
		t.Fatalf("write websocket message: %v", err)
	}

	var result struct {
		Type        string `json:"type"`
		Message     string `json:"message"`
		ShortAnswer string `json:"short_answer"`
		RunStatus   string `json:"run_status"`
		Steps       []struct {
			ToolName string `json:"tool_name"`
			Summary  string `json:"summary"`
		} `json:"steps"`
	}
	for attempts := 0; attempts < 4; attempts++ {
		if err := connection.ReadJSON(&result); err != nil {
			t.Fatalf("read websocket response: %v", err)
		}
		if result.Type == "assistant_message" {
			break
		}
	}
	if result.Type != "assistant_message" {
		t.Fatalf("expected assistant_message, got %#v", result)
	}
	if result.RunStatus != "completed" {
		t.Fatalf("expected completed run status, got %#v", result)
	}
	if len(result.Steps) != 4 {
		t.Fatalf("expected 4 investigation steps, got %#v", result)
	}
	if result.Steps[1].ToolName != "query.access.search" {
		t.Fatalf("expected access enrichment step, got %#v", result)
	}
	if !strings.Contains(result.Message, "later accepted") {
		t.Fatalf("expected correlated acceptance in message, got %#v", result)
	}
	if !strings.Contains(result.Message, "started session sess-001") {
		t.Fatalf("expected session correlation in message, got %#v", result)
	}
	if !strings.HasPrefix(result.Message, "Yes.") {
		t.Fatalf("expected direct answer prefix in message, got %#v", result)
	}
	if !strings.Contains(result.ShortAnswer, "Authentication failed before the subscriber connected.") {
		t.Fatalf("expected concise short answer, got %#v", result)
	}
	if !strings.Contains(result.Steps[0].Summary, `query "john authentication rejected accepted connected session start invalid password"`) {
		t.Fatalf("expected bounded auth query rewrite, got %#v", result)
	}

	if err := connection.WriteJSON(map[string]any{
		"type":      "ask",
		"tenant_id": "default",
		"message":   "How is John?",
		"filters": map[string]any{
			"subscriber_id": "john",
		},
		"max_steps": 4,
	}); err != nil {
		t.Fatalf("write websocket status message: %v", err)
	}

	var statusResult struct {
		Type        string `json:"type"`
		Message     string `json:"message"`
		ShortAnswer string `json:"short_answer"`
		RunStatus   string `json:"run_status"`
		Steps       []struct {
			ToolName string `json:"tool_name"`
			Summary  string `json:"summary"`
		} `json:"steps"`
	}
	for attempts := 0; attempts < 4; attempts++ {
		if err := connection.ReadJSON(&statusResult); err != nil {
			t.Fatalf("read websocket status response: %v", err)
		}
		if statusResult.Type == "assistant_message" {
			break
		}
	}
	if statusResult.Type != "assistant_message" {
		t.Fatalf("expected assistant_message for status question, got %#v", statusResult)
	}
	if !strings.HasPrefix(statusResult.ShortAnswer, "Subscriber john is not currently connected") {
		t.Fatalf("expected concise status short answer, got %#v", statusResult)
	}
	if strings.HasPrefix(statusResult.ShortAnswer, "Investigated goal") {
		t.Fatalf("expected status short answer instead of generic summary, got %#v", statusResult)
	}
	if len(statusResult.Steps) != 3 {
		t.Fatalf("expected lighter three-step status route, got %#v", statusResult)
	}
	if statusResult.Steps[0].ToolName != "query.sessions.search" {
		t.Fatalf("expected status route to start with session enrichment, got %#v", statusResult)
	}

	if err := connection.WriteJSON(map[string]any{
		"type":      "ask",
		"tenant_id": "default",
		"message":   "Does john belong to this tenant?",
		"filters": map[string]any{
			"subscriber_id": "john",
		},
		"max_steps": 4,
	}); err != nil {
		t.Fatalf("write websocket tenant message: %v", err)
	}

	var tenantResult struct {
		Type        string `json:"type"`
		Message     string `json:"message"`
		ShortAnswer string `json:"short_answer"`
		RunStatus   string `json:"run_status"`
		Steps       []struct {
			ToolName string `json:"tool_name"`
			Summary  string `json:"summary"`
		} `json:"steps"`
	}
	for attempts := 0; attempts < 4; attempts++ {
		if err := connection.ReadJSON(&tenantResult); err != nil {
			t.Fatalf("read websocket tenant response: %v", err)
		}
		if tenantResult.Type == "assistant_message" {
			break
		}
	}
	if tenantResult.Type != "assistant_message" {
		t.Fatalf("expected assistant_message for tenant question, got %#v", tenantResult)
	}
	if !strings.HasPrefix(tenantResult.ShortAnswer, "Yes. Subscriber john has evidence in this tenant.") {
		t.Fatalf("expected concise tenant short answer, got %#v", tenantResult)
	}
	if strings.HasPrefix(tenantResult.ShortAnswer, "Investigated goal") {
		t.Fatalf("expected tenant short answer instead of generic summary, got %#v", tenantResult)
	}
	if len(tenantResult.Steps) != 2 || !strings.Contains(tenantResult.Steps[0].Summary, `query "john subscriber tenant membership"`) {
		t.Fatalf("expected lighter retrieval-only tenant route, got %#v", tenantResult)
	}

	if err := connection.WriteJSON(map[string]any{
		"type":      "ask",
		"tenant_id": "default",
		"message":   "Who manages John?",
		"filters": map[string]any{
			"subscriber_id": "john",
		},
		"max_steps": 4,
	}); err != nil {
		t.Fatalf("write websocket unsupported message: %v", err)
	}

	var unsupportedResult struct {
		Type        string `json:"type"`
		Message     string `json:"message"`
		ShortAnswer string `json:"short_answer"`
		RunStatus   string `json:"run_status"`
	}
	for attempts := 0; attempts < 4; attempts++ {
		if err := connection.ReadJSON(&unsupportedResult); err != nil {
			t.Fatalf("read websocket unsupported response: %v", err)
		}
		if unsupportedResult.Type == "assistant_message" {
			break
		}
	}
	if unsupportedResult.Type != "assistant_message" {
		t.Fatalf("expected assistant_message for unsupported question, got %#v", unsupportedResult)
	}
	if !strings.HasPrefix(unsupportedResult.ShortAnswer, "I don't have a grounded answer template for that question yet.") {
		t.Fatalf("expected bounded unsupported-question fallback, got %#v", unsupportedResult)
	}
	if strings.HasPrefix(unsupportedResult.ShortAnswer, "Investigated goal") {
		t.Fatalf("expected unsupported-question short answer instead of generic summary, got %#v", unsupportedResult)
	}

	a2aAuthResponse, err := http.Post(
		httpServer.URL+"/v1/a2a/messages",
		"application/json",
		strings.NewReader(`{"tenant_id":"default","message":"Did john fail auth before connecting?","filters":{"subscriber_id":"john"},"max_steps":4}`),
	)
	if err != nil {
		t.Fatalf("POST /v1/a2a/messages auth error = %v", err)
	}
	defer a2aAuthResponse.Body.Close()
	if a2aAuthResponse.StatusCode != http.StatusOK {
		t.Fatalf("a2a auth status = %d, want %d", a2aAuthResponse.StatusCode, http.StatusOK)
	}

	var a2aAuthResult struct {
		Type        string `json:"type"`
		Message     string `json:"message"`
		ShortAnswer string `json:"short_answer"`
		RunStatus   string `json:"run_status"`
		Steps       []struct {
			ToolName string `json:"tool_name"`
			Summary  string `json:"summary"`
		} `json:"steps"`
	}
	if err := json.NewDecoder(a2aAuthResponse.Body).Decode(&a2aAuthResult); err != nil {
		t.Fatalf("decode a2a auth response: %v", err)
	}
	if a2aAuthResult.Type != "assistant_message" || a2aAuthResult.RunStatus != "completed" {
		t.Fatalf("expected completed a2a auth response, got %#v", a2aAuthResult)
	}
	if len(a2aAuthResult.Steps) != 4 {
		t.Fatalf("expected full four-step cross-source a2a route, got %#v", a2aAuthResult)
	}
	if !strings.Contains(a2aAuthResult.Message, "later accepted") {
		t.Fatalf("expected correlated auth answer from a2a, got %#v", a2aAuthResult)
	}

	a2aStatusResponse, err := http.Post(
		httpServer.URL+"/v1/a2a/messages",
		"application/json",
		strings.NewReader(`{"tenant_id":"default","message":"How is John?","filters":{"subscriber_id":"john"},"max_steps":4}`),
	)
	if err != nil {
		t.Fatalf("POST /v1/a2a/messages status error = %v", err)
	}
	defer a2aStatusResponse.Body.Close()
	if a2aStatusResponse.StatusCode != http.StatusOK {
		t.Fatalf("a2a status status = %d, want %d", a2aStatusResponse.StatusCode, http.StatusOK)
	}

	var a2aStatusResult struct {
		Type        string `json:"type"`
		ShortAnswer string `json:"short_answer"`
		RunStatus   string `json:"run_status"`
		Steps       []struct {
			ToolName string `json:"tool_name"`
			Summary  string `json:"summary"`
		} `json:"steps"`
	}
	if err := json.NewDecoder(a2aStatusResponse.Body).Decode(&a2aStatusResult); err != nil {
		t.Fatalf("decode a2a status response: %v", err)
	}
	if a2aStatusResult.Type != "assistant_message" || a2aStatusResult.RunStatus != "completed" {
		t.Fatalf("expected completed a2a status response, got %#v", a2aStatusResult)
	}
	if !strings.HasPrefix(a2aStatusResult.ShortAnswer, "Subscriber john is not currently connected") {
		t.Fatalf("expected concise a2a status answer, got %#v", a2aStatusResult)
	}
	if len(a2aStatusResult.Steps) != 3 || a2aStatusResult.Steps[0].ToolName != "query.sessions.search" {
		t.Fatalf("expected lighter three-step a2a status route, got %#v", a2aStatusResult)
	}
}

func TestNewServerSearchesIndexedRetrievalDocuments(t *testing.T) {
	metadataPath := t.TempDir()
	store, err := retrievalmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	occurred := time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC)
	document := retrievalcontracts.Document{
		DocumentID:      "evt-001",
		SchemaVersion:   retrievalcontracts.SchemaVersion,
		TenantID:        "default",
		Source:          "ras",
		SourceEventID:   "raw-001",
		SourceKey:       "radius:acct:john:sess-001:20260505T140000.000000",
		EntityType:      retrievalcontracts.EntityTypeNetworkSession,
		EventType:       unifiedsessions.EventType,
		OccurredAt:      occurred,
		IngestedAt:      occurred.Add(time.Minute),
		Title:           "network session stop for subscriber john in session sess-001 on NAS 192.168.1.1",
		Content:         "user john disconnected from NAS 192.168.1.1 after a 100 second session",
		SubscriberID:    "john",
		SessionID:       "sess-001",
		Status:          "stop",
		NASIPAddress:    "192.168.1.1",
		ClientIPAddress: "10.0.0.25",
		Terms: retrievalcontracts.NormalizeTerms(
			"network session stop for subscriber john in session sess-001 on NAS 192.168.1.1",
			"user john disconnected from NAS 192.168.1.1 after a 100 second session",
			"john",
			"sess-001",
			"stop",
			"192.168.1.1",
			"10.0.0.25",
		),
	}
	if err := store.Append(context.Background(), document); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	cfg := &silconfig.Config{
		API: silconfig.APIConfig{
			ListenAddr:   ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
		Storage: silconfig.StorageConfig{
			Metadata: silconfig.StoreConfig{
				Backend: "file",
				Path:    metadataPath,
			},
		},
	}

	server, err := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/retrieve", strings.NewReader(`{"tenant_id":"default","query":"john disconnected","limit":5}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("retrieval status = %d, want %d", response.Code, http.StatusOK)
	}

	var payload struct {
		Count int                      `json:"count"`
		Hits  []retrievalcontracts.Hit `json:"hits"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode retrieval response: %v", err)
	}
	if payload.Count != 1 || len(payload.Hits) != 1 {
		t.Fatalf("unexpected retrieval payload: %+v", payload)
	}
	if payload.Hits[0].Document.SessionID != "sess-001" {
		t.Fatalf("expected retrieval hit for sess-001, got %q", payload.Hits[0].Document.SessionID)
	}
}

func TestNewServerSearchesIndexedAccessRetrievalDocuments(t *testing.T) {
	metadataPath := t.TempDir()
	store, err := retrievalmetadata.NewStore(silconfig.StoreConfig{
		Backend: "file",
		Path:    metadataPath,
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	occurred := time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC)
	document := retrievalcontracts.Document{
		DocumentID:      "evt-access-001",
		SchemaVersion:   retrievalcontracts.SchemaVersion,
		TenantID:        "default",
		Source:          "aaa_auth",
		SourceEventID:   "raw-access-001",
		SourceKey:       "access:auth:john:req-001:20260505T140000.000000",
		EntityType:      retrievalcontracts.EntityTypeNetworkAccess,
		EventType:       unifiedaccess.EventType,
		OccurredAt:      occurred,
		IngestedAt:      occurred.Add(time.Minute),
		Title:           "network access rejected for subscriber john on request req-001 on NAS 192.168.1.1",
		Content:         "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
		SubscriberID:    "john",
		SessionID:       "sess-001",
		RequestID:       "req-001",
		Outcome:         "rejected",
		NASIPAddress:    "192.168.1.1",
		ClientIPAddress: "10.0.0.25",
		RejectReason:    "invalid password",
		Terms: retrievalcontracts.NormalizeTerms(
			"network access rejected for subscriber john on request req-001 on NAS 192.168.1.1",
			"user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
			"john",
			"req-001",
			"rejected",
			"invalid password",
		),
	}
	if err := store.Append(context.Background(), document); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	cfg := &silconfig.Config{
		API: silconfig.APIConfig{
			ListenAddr:   ":8080",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
		Storage: silconfig.StorageConfig{
			Metadata: silconfig.StoreConfig{
				Backend: "file",
				Path:    metadataPath,
			},
		},
	}

	server, err := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/retrieve", strings.NewReader(`{"tenant_id":"default","query":"invalid password","outcome":"rejected","limit":5}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("retrieval status = %d, want %d", response.Code, http.StatusOK)
	}

	var payload struct {
		Count int                      `json:"count"`
		Hits  []retrievalcontracts.Hit `json:"hits"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode retrieval response: %v", err)
	}
	if payload.Count != 1 || len(payload.Hits) != 1 {
		t.Fatalf("unexpected retrieval payload: %+v", payload)
	}
	if payload.Hits[0].Document.RequestID != "req-001" || payload.Hits[0].Document.EntityType != retrievalcontracts.EntityTypeNetworkAccess {
		t.Fatalf("unexpected access retrieval hit: %+v", payload.Hits[0].Document)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
