package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/asimarora/semantic-intelligence-layer/internal/agents/harness"
	agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
	assistanttransport "github.com/asimarora/semantic-intelligence-layer/internal/transport/assistant"
)

const defaultMessageTimeout = 30 * time.Second

type Dependencies struct {
	Logger  *slog.Logger
	Harness *harness.Service
}

type Handler struct {
	logger  *slog.Logger
	harness *harness.Service
}

func NewHandler(deps Dependencies) (*Handler, error) {
	if deps.Harness == nil {
		return nil, fmt.Errorf("a2a harness is required")
	}
	return &Handler{
		logger:  deps.Logger,
		harness: deps.Harness,
	}, nil
}

func (handler *Handler) ServeMessage(w http.ResponseWriter, r *http.Request) {
	if handler == nil || handler.harness == nil {
		writeJSON(w, http.StatusServiceUnavailable, assistanttransport.MessageResponse{
			Type:      "error",
			Error:     "a2a interface is not configured",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	var message assistanttransport.MessageRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil {
		writeJSON(w, http.StatusBadRequest, assistanttransport.BuildErrorResponse(fmt.Errorf("invalid request body: %w", err), agentmetadata.Record{}))
		return
	}

	runRequest, err := assistanttransport.BuildRunRequest(message, assistanttransport.MinMaxSteps)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, assistanttransport.BuildErrorResponse(err, agentmetadata.Record{}))
		return
	}

	if handler.logger != nil {
		handler.logger.Info("handling a2a message", "tenant_id", runRequest.TenantID, "goal", runRequest.Goal)
	}

	ctx, cancel := context.WithTimeout(r.Context(), defaultMessageTimeout)
	defer cancel()

	record, runErr := handler.harness.RunInvestigation(ctx, runRequest)
	if runErr != nil {
		writeJSON(w, http.StatusInternalServerError, assistanttransport.BuildErrorResponse(runErr, record))
		return
	}
	writeJSON(w, http.StatusOK, assistanttransport.BuildResultResponse(record))
}

func writeJSON(w http.ResponseWriter, statusCode int, payload assistanttransport.MessageResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
