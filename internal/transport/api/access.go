package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
	queryservice "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
	accessstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/access"
)

type accessResponse struct {
	Count  int                   `json:"count"`
	Events []unifiedaccess.Event `json:"events"`
}

func registerAccessRoutes(mux *http.ServeMux, logger *slog.Logger, service *queryservice.AccessService, initErr error) {
	mux.HandleFunc("/v1/access-events", func(w http.ResponseWriter, r *http.Request) {
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
			writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("access query service is not configured"))
			return
		}

		query, err := parseAccessQuery(r)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		events, err := service.SearchNetworkAccessEvents(r.Context(), query)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}

		if logger != nil {
			logger.Info("served access query", "tenant_id", query.TenantID, "count", len(events))
		}
		writeAPIJSON(w, http.StatusOK, accessResponse{
			Count:  len(events),
			Events: events,
		})
	})
}

func parseAccessQuery(r *http.Request) (accessstore.Query, error) {
	values := r.URL.Query()
	from, err := parseTimeValue(values.Get("from"))
	if err != nil {
		return accessstore.Query{}, fmt.Errorf("invalid from: %w", err)
	}
	to, err := parseTimeValue(values.Get("to"))
	if err != nil {
		return accessstore.Query{}, fmt.Errorf("invalid to: %w", err)
	}
	if from != nil && to != nil && from.After(*to) {
		return accessstore.Query{}, fmt.Errorf("from must be before or equal to to")
	}

	limit, err := parsePositiveInt(values.Get("limit"))
	if err != nil {
		return accessstore.Query{}, fmt.Errorf("invalid limit: %w", err)
	}

	query := accessstore.Query{
		TenantID:        strings.TrimSpace(values.Get("tenant_id")),
		SubscriberID:    strings.TrimSpace(values.Get("subscriber_id")),
		RequestID:       strings.TrimSpace(values.Get("request_id")),
		SessionID:       strings.TrimSpace(values.Get("session_id")),
		Outcome:         strings.TrimSpace(values.Get("outcome")),
		NASIPAddress:    strings.TrimSpace(values.Get("nas_ip_address")),
		ClientIPAddress: strings.TrimSpace(values.Get("client_ip_address")),
		From:            from,
		To:              to,
		Limit:           limit,
	}
	if query.TenantID == "" {
		return accessstore.Query{}, fmt.Errorf("tenant_id is required")
	}
	return query, nil
}
