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

	agenttypes "github.com/asimarora/semantic-intelligence-layer/internal/agents"
	"github.com/asimarora/semantic-intelligence-layer/internal/agents/harness"
	agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
	"github.com/gorilla/websocket"
)

const (
	defaultChatRunTimeout = 30 * time.Second
	maxChatMessageBytes   = 1 << 20
	minChatMaxSteps       = 4
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

type clientMessage struct {
	Type         string        `json:"type,omitempty"`
	TenantID     string        `json:"tenant_id"`
	CaseID       string        `json:"case_id,omitempty"`
	RunSessionID string        `json:"run_session_id,omitempty"`
	Message      string        `json:"message"`
	Filters      clientFilters `json:"filters,omitempty"`
	MaxSteps     int           `json:"max_steps,omitempty"`
}

type clientFilters struct {
	SubscriberID    string `json:"subscriber_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
	Status          string `json:"status,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
	NASIPAddress    string `json:"nas_ip_address,omitempty"`
	ClientIPAddress string `json:"client_ip_address,omitempty"`
	From            string `json:"from,omitempty"`
	To              string `json:"to,omitempty"`
	Limit           int    `json:"limit,omitempty"`
}

type serverMessage struct {
	Type        string        `json:"type"`
	Message     string        `json:"message,omitempty"`
	ShortAnswer string        `json:"short_answer,omitempty"`
	Error       string        `json:"error,omitempty"`
	RunID       string        `json:"run_id,omitempty"`
	RunStatus   string        `json:"run_status,omitempty"`
	Goal        string        `json:"goal,omitempty"`
	Steps       []stepPayload `json:"steps,omitempty"`
	StartedAt   string        `json:"started_at,omitempty"`
	CompletedAt string        `json:"completed_at,omitempty"`
	Timestamp   string        `json:"timestamp"`
}

type stepPayload struct {
	Kind          string `json:"kind"`
	ToolName      string `json:"tool_name"`
	Status        string `json:"status"`
	Summary       string `json:"summary"`
	EvidenceCount int    `json:"evidence_count"`
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
	if err := handler.writeMessage(connection, serverMessage{
		Type:      "welcome",
		Message:   "Connected. Ask a bounded investigation question to run against the current evidence.",
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return
	}

	for {
		var inbound clientMessage
		if err := connection.ReadJSON(&inbound); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				handler.log().Error("chat websocket closed unexpectedly", "error", err)
			}
			return
		}

		request, err := buildRunRequest(inbound)
		if err != nil {
			if writeErr := handler.writeError(connection, err, agentmetadata.Record{}); writeErr != nil {
				return
			}
			continue
		}

		if err := handler.writeMessage(connection, serverMessage{
			Type:      "user_message",
			Message:   request.Goal,
			Goal:      request.Goal,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return
		}
		if err := handler.writeMessage(connection, serverMessage{
			Type:      "status",
			Message:   "Running deterministic investigation against indexed, access, and session evidence...",
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

func buildRunRequest(message clientMessage) (agenttypes.RunRequest, error) {
	messageType := strings.ToLower(strings.TrimSpace(message.Type))
	if messageType != "" && messageType != "message" && messageType != "ask" {
		return agenttypes.RunRequest{}, fmt.Errorf("unsupported chat message type %q", message.Type)
	}

	tenantID := strings.TrimSpace(message.TenantID)
	if tenantID == "" {
		return agenttypes.RunRequest{}, fmt.Errorf("tenant_id is required")
	}

	text := strings.TrimSpace(message.Message)
	if text == "" {
		return agenttypes.RunRequest{}, fmt.Errorf("message is required")
	}

	inputs := map[string]string{
		"query": text,
	}
	addInput(inputs, "subscriber_id", message.Filters.SubscriberID)
	addInput(inputs, "session_id", message.Filters.SessionID)
	addInput(inputs, "request_id", message.Filters.RequestID)
	addInput(inputs, "status", message.Filters.Status)
	addInput(inputs, "outcome", message.Filters.Outcome)
	addInput(inputs, "nas_ip_address", message.Filters.NASIPAddress)
	addInput(inputs, "client_ip_address", message.Filters.ClientIPAddress)
	addInput(inputs, "from", message.Filters.From)
	addInput(inputs, "to", message.Filters.To)
	if message.Filters.Limit > 0 {
		inputs["limit"] = fmt.Sprintf("%d", message.Filters.Limit)
	}

	maxSteps := message.MaxSteps
	if maxSteps < minChatMaxSteps {
		maxSteps = minChatMaxSteps
	}

	return agenttypes.RunRequest{
		TenantID:  tenantID,
		CaseID:    strings.TrimSpace(message.CaseID),
		SessionID: strings.TrimSpace(message.RunSessionID),
		Goal:      text,
		Inputs:    inputs,
		MaxSteps:  maxSteps,
	}, nil
}

func addInput(inputs map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	inputs[key] = value
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
	message := serverMessage{
		Type:        "error",
		Error:       err.Error(),
		ShortAnswer: strings.TrimSpace(err.Error()),
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if record.Run.ID != "" {
		message.RunID = record.Run.ID
		message.RunStatus = string(record.Run.Status)
		message.Goal = record.Run.Goal
		message.Steps = mapSteps(record.Steps)
		message.Message = latestStepSummary(record)
		message.ShortAnswer = extractShortAnswer(message.Message)
	}
	return handler.writeMessage(connection, message)
}

func (handler *Handler) writeResult(connection *websocket.Conn, record agentmetadata.Record) error {
	startedAt := record.Run.StartedAt.UTC().Format(time.RFC3339Nano)
	completedAt := ""
	if record.Run.CompletedAt != nil {
		completedAt = record.Run.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	summary := latestStepSummary(record)
	return handler.writeMessage(connection, serverMessage{
		Type:        "assistant_message",
		Message:     summary,
		ShortAnswer: extractShortAnswer(summary),
		RunID:       record.Run.ID,
		RunStatus:   string(record.Run.Status),
		Goal:        record.Run.Goal,
		Steps:       mapSteps(record.Steps),
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (handler *Handler) writeMessage(connection *websocket.Conn, payload serverMessage) error {
	if connection == nil {
		return io.ErrClosedPipe
	}
	if err := connection.SetWriteDeadline(time.Now().UTC().Add(5 * time.Second)); err != nil {
		return err
	}
	return connection.WriteJSON(payload)
}

func latestStepSummary(record agentmetadata.Record) string {
	for index := len(record.Steps) - 1; index >= 0; index-- {
		summary := strings.TrimSpace(record.Steps[index].Summary)
		if summary != "" {
			return summary
		}
	}
	if strings.TrimSpace(record.Run.Goal) != "" {
		return fmt.Sprintf("Investigation for %q completed with status %s.", record.Run.Goal, record.Run.Status)
	}
	return fmt.Sprintf("Investigation completed with status %s.", record.Run.Status)
}

func extractShortAnswer(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	sentences := splitSentences(summary)
	if len(sentences) == 0 {
		return summary
	}
	first := sentences[0]
	if (first == "Yes." || first == "No." || first == "Partially.") && len(sentences) > 1 {
		return first + " " + sentences[1]
	}
	return first
}

func splitSentences(text string) []string {
	sentences := make([]string, 0, 4)
	start := 0
	for index, r := range text {
		if r != '.' && r != '!' && r != '?' {
			continue
		}
		sentence := strings.TrimSpace(text[start : index+1])
		if sentence != "" {
			sentences = append(sentences, sentence)
		}
		start = index + 1
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		sentences = append(sentences, tail)
	}
	return sentences
}

func mapSteps(steps []agenttypes.Step) []stepPayload {
	mapped := make([]stepPayload, 0, len(steps))
	for _, step := range steps {
		mapped = append(mapped, stepPayload{
			Kind:          step.Kind,
			ToolName:      step.ToolName,
			Status:        string(step.Status),
			Summary:       step.Summary,
			EvidenceCount: len(step.Evidence),
		})
	}
	return mapped
}

func (handler *Handler) log() *slog.Logger {
	if handler != nil && handler.logger != nil {
		return handler.logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
