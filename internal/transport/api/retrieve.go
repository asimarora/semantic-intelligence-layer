package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
	queryservice "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
	retrievalstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
)

type retrievalRequest struct {
	TenantID        string `json:"tenant_id"`
	Query           string `json:"query"`
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

type retrievalResponse struct {
	Count int                      `json:"count"`
	Hits  []retrievalcontracts.Hit `json:"hits"`
}

func registerRetrievalRoutes(mux *http.ServeMux, logger *slog.Logger, service *queryservice.RetrievalService, initErr error) {
	mux.HandleFunc("/v1/retrieve", func(w http.ResponseWriter, r *http.Request) {
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
			writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("retrieval query service is not configured"))
			return
		}

		filter, err := parseRetrievalQuery(r)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		hits, err := service.SearchEvidence(r.Context(), filter)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}

		if logger != nil {
			logger.Info(
				"served retrieval query",
				"tenant_id", filter.TenantID,
				"query", filter.QueryText,
				"count", len(hits),
			)
		}
		writeAPIJSON(w, http.StatusOK, retrievalResponse{
			Count: len(hits),
			Hits:  hits,
		})
	})
}

func parseRetrievalQuery(r *http.Request) (retrievalstore.Query, error) {
	var request retrievalRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return retrievalstore.Query{}, fmt.Errorf("invalid request body: %w", err)
	}

	from, err := parseTimeValue(request.From)
	if err != nil {
		return retrievalstore.Query{}, fmt.Errorf("invalid from: %w", err)
	}
	to, err := parseTimeValue(request.To)
	if err != nil {
		return retrievalstore.Query{}, fmt.Errorf("invalid to: %w", err)
	}
	if from != nil && to != nil && from.After(*to) {
		return retrievalstore.Query{}, fmt.Errorf("from must be before or equal to to")
	}

	filter := retrievalstore.Query{
		TenantID:        strings.TrimSpace(request.TenantID),
		QueryText:       strings.TrimSpace(request.Query),
		SubscriberID:    strings.TrimSpace(request.SubscriberID),
		SessionID:       strings.TrimSpace(request.SessionID),
		RequestID:       strings.TrimSpace(request.RequestID),
		Status:          strings.TrimSpace(request.Status),
		Outcome:         strings.TrimSpace(request.Outcome),
		NASIPAddress:    strings.TrimSpace(request.NASIPAddress),
		ClientIPAddress: strings.TrimSpace(request.ClientIPAddress),
		From:            from,
		To:              to,
		Limit:           request.Limit,
	}
	if filter.TenantID == "" {
		return retrievalstore.Query{}, fmt.Errorf("tenant_id is required")
	}
	if filter.QueryText == "" {
		return retrievalstore.Query{}, fmt.Errorf("query is required")
	}
	if filter.Limit < 0 {
		return retrievalstore.Query{}, fmt.Errorf("limit must be a positive integer")
	}
	return filter, nil
}
