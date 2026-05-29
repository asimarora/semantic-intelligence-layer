package chat

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/asimarora/semantic-intelligence-layer/internal/agents/harness"
	agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
	assistanttransport "github.com/asimarora/semantic-intelligence-layer/internal/transport/assistant"
	"github.com/gorilla/websocket"
)

const (
	defaultChatRunTimeout = 30 * time.Second
	maxChatMessageBytes   = 1 << 20
)

//go:embed assets/index.html
var pageHTML []byte

type Dependencies struct {
	Logger  *slog.Logger
	Harness *harness.Service
}

type Handler struct {
	logger   *slog.Logger
	harness  *harness.Service
	upgrader websocket.Upgrader
}

func NewHandler(deps Dependencies) (*Handler, error) {
	if deps.Harness == nil {
		return nil, fmt.Errorf("chat harness is required")
	}
	return &Handler{
		logger:  deps.Logger,
		harness: deps.Harness,
		upgrader: websocket.Upgrader{
			CheckOrigin: allowWebSocketOrigin,
		},
	}, nil
}

func (handler *Handler) ServePage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pageHTML)
}

func (handler *Handler) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	if handler == nil || handler.harness == nil {
		http.Error(w, "chat interface is not configured", http.StatusServiceUnavailable)
		return
	}

	connection, err := handler.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()

	connection.SetReadLimit(maxChatMessageBytes)
	if err := handler.writeMessage(connection, assistanttransport.MessageResponse{
		Type:      "welcome",
		Message:   "Connected. Ask a bounded investigation question to run against the current evidence.",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return
	}

	for {
		var inbound assistanttransport.MessageRequest
		if err := connection.ReadJSON(&inbound); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				handler.log().Error("chat websocket closed unexpectedly", "error", err)
			}
			return
		}

		request, err := assistanttransport.BuildRunRequest(inbound, assistanttransport.MinMaxSteps)
		if err != nil {
			if writeErr := handler.writeError(connection, err, agentmetadata.Record{}); writeErr != nil {
				return
			}
			continue
		}

		if err := handler.writeMessage(connection, assistanttransport.MessageResponse{
			Type:      "user_message",
			Message:   request.Goal,
			Goal:      request.Goal,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return
		}
		if err := handler.writeMessage(connection, assistanttransport.MessageResponse{
			Type:      "status",
			Message:   "Running deterministic investigation against the current evidence...",
			Goal:      request.Goal,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return
		}

		runCtx, cancel := context.WithTimeout(r.Context(), defaultChatRunTimeout)
		record, err := handler.harness.RunInvestigation(runCtx, request)
		cancel()
		if err != nil {
			if writeErr := handler.writeError(connection, err, record); writeErr != nil {
				return
			}
			continue
		}
		if err := handler.writeResult(connection, record); err != nil {
			return
		}
	}
}

func allowWebSocketOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func (handler *Handler) writeError(connection *websocket.Conn, err error, record agentmetadata.Record) error {
	message := assistanttransport.BuildErrorResponse(err, record)
	message.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	return handler.writeMessage(connection, message)
}

func (handler *Handler) writeResult(connection *websocket.Conn, record agentmetadata.Record) error {
	message := assistanttransport.BuildResultResponse(record)
	message.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	return handler.writeMessage(connection, message)
}

func (handler *Handler) writeMessage(connection *websocket.Conn, payload assistanttransport.MessageResponse) error {
	if connection == nil {
		return io.ErrClosedPipe
	}
	if err := connection.SetWriteDeadline(time.Now().UTC().Add(5 * time.Second)); err != nil {
		return err
	}
	return connection.WriteJSON(payload)
}

func (handler *Handler) log() *slog.Logger {
	if handler != nil && handler.logger != nil {
		return handler.logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
