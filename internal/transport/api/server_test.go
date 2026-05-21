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

        retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
        unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
        retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
        sessionmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
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

func writeFile(t *testing.T, path, content string) {
        t.Helper()
        if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
                t.Fatalf("write %s: %v", path, err)
        }
}
