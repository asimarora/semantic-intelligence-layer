package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	agenttypes "github.com/asimarora/semantic-intelligence-layer/internal/agents"
	"github.com/asimarora/semantic-intelligence-layer/internal/agents/harness"
	agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
)

type agentRunRequest struct {
	TenantID     string          `json:"tenant_id"`
	CaseID       string          `json:"case_id,omitempty"`
	RunSessionID string          `json:"run_session_id,omitempty"`
	Goal         string          `json:"goal,omitempty"`
	Query        string          `json:"query,omitempty"`
	Filters      agentRunFilters `json:"filters,omitempty"`
	MaxSteps     int             `json:"max_steps,omitempty"`
}

type agentRunFilters struct {
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

type agentRunResponse struct {
	Run   agenttypes.Run    `json:"run"`
	Steps []agenttypes.Step `json:"steps"`
}

func registerAgentRoutes(mux *http.ServeMux, logger *slog.Logger, service *harness.Service, initErr error) {
	mux.HandleFunc("/v1/agent-runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method %s is not allowed", r.Method))
			return
		}
		if initErr != nil {
			writeAPIError(w, http.StatusNotImplemented, initErr)
			return
		}
		if service == nil {
			writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("agent harness is not configured"))
			return
		}

		request, err := parseAgentRunRequest(r)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		record, err := service.RunInvestigation(r.Context(), request)
		if err != nil {
			switch {
			case errors.Is(err, harness.ErrApprovalRequired):
				writeAgentRunJSON(w, http.StatusAccepted, record)
			case errors.Is(err, harness.ErrPolicyDenied):
				writeAPIError(w, http.StatusForbidden, err)
			default:
				writeAPIError(w, http.StatusInternalServerError, err)
			}
			return
		}

		if logger != nil {
			logger.Info(
				"completed agent investigation run",
				"run_id", record.Run.ID,
				"tenant_id", record.Run.TenantID,
				"status", record.Run.Status,
			)
		}
		writeAgentRunJSON(w, http.StatusOK, record)
	})

	mux.HandleFunc("/v1/agent-runs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method %s is not allowed", r.Method))
			return
		}
		if initErr != nil {
			writeAPIError(w, http.StatusNotImplemented, initErr)
			return
		}
		if service == nil {
			writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("agent harness is not configured"))
			return
		}

		runID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/agent-runs/"))
		if runID == "" || strings.Contains(runID, "/") {
			writeAPIError(w, http.StatusBadRequest, fmt.Errorf("run id is required"))
			return
		}

		record, err := service.GetRun(r.Context(), runID)
		if err != nil {
			switch {
			case errors.Is(err, agentmetadata.ErrNotFound):
				writeAPIError(w, http.StatusNotFound, err)
			default:
				writeAPIError(w, http.StatusInternalServerError, err)
			}
			return
		}
		writeAgentRunJSON(w, http.StatusOK, record)
	})
}

func parseAgentRunRequest(r *http.Request) (agenttypes.RunRequest, error) {
	var payload agentRunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return agenttypes.RunRequest{}, fmt.Errorf("invalid request body: %w", err)
	}

	inputs := make(map[string]string)
	if query := strings.TrimSpace(payload.Query); query != "" {
		inputs["query"] = query
	}
	if subscriberID := strings.TrimSpace(payload.Filters.SubscriberID); subscriberID != "" {
		inputs["subscriber_id"] = subscriberID
	}
	if sessionID := strings.TrimSpace(payload.Filters.SessionID); sessionID != "" {
		inputs["session_id"] = sessionID
	}
	if requestID := strings.TrimSpace(payload.Filters.RequestID); requestID != "" {
		inputs["request_id"] = requestID
	}
	if status := strings.TrimSpace(payload.Filters.Status); status != "" {
		inputs["status"] = status
	}
	if outcome := strings.TrimSpace(payload.Filters.Outcome); outcome != "" {
		inputs["outcome"] = outcome
	}
	if nasIPAddress := strings.TrimSpace(payload.Filters.NASIPAddress); nasIPAddress != "" {
		inputs["nas_ip_address"] = nasIPAddress
	}
	if clientIPAddress := strings.TrimSpace(payload.Filters.ClientIPAddress); clientIPAddress != "" {
		inputs["client_ip_address"] = clientIPAddress
	}

	from, err := parseTimeValue(payload.Filters.From)
	if err != nil {
		return agenttypes.RunRequest{}, fmt.Errorf("invalid filters.from: %w", err)
	}
	if to, err := parseTimeValue(payload.Filters.To); err != nil {
		return agenttypes.RunRequest{}, fmt.Errorf("invalid filters.to: %w", err)
	} else if from != nil && to != nil && from.After(*to) {
		return agenttypes.RunRequest{}, fmt.Errorf("filters.from must be before or equal to filters.to")
	} else if to != nil {
		inputs["to"] = to.Format(time.RFC3339)
	}
	if from != nil {
		inputs["from"] = from.Format(time.RFC3339)
	}

	if payload.Filters.Limit < 0 {
		return agenttypes.RunRequest{}, fmt.Errorf("filters.limit must be a positive integer")
	}
	if payload.Filters.Limit > 0 {
		inputs["limit"] = strconv.Itoa(payload.Filters.Limit)
	}

	if strings.TrimSpace(payload.TenantID) == "" {
		return agenttypes.RunRequest{}, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(payload.Goal) == "" && strings.TrimSpace(payload.Query) == "" {
		return agenttypes.RunRequest{}, fmt.Errorf("goal or query is required")
	}

	return agenttypes.RunRequest{
		TenantID:  strings.TrimSpace(payload.TenantID),
		CaseID:    strings.TrimSpace(payload.CaseID),
		SessionID: strings.TrimSpace(payload.RunSessionID),
		Goal:      strings.TrimSpace(payload.Goal),
		Inputs:    inputs,
		MaxSteps:  payload.MaxSteps,
	}, nil
}

func writeAgentRunJSON(w http.ResponseWriter, status int, record agentmetadata.Record) {
	writeAPIJSON(w, status, agentRunResponse{
		Run:   record.Run,
		Steps: record.Steps,
	})
}
