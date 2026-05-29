package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
	queryservice "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
	sessionstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
)

type sessionsResponse struct {
	Count    int                     `json:"count"`
	Sessions []unifiedsessions.Event `json:"sessions"`
}

func registerSessionRoutes(mux *http.ServeMux, logger *slog.Logger, service *queryservice.SessionService, initErr error) {
	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
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
			writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("session query service is not configured"))
			return
		}

		query, err := parseSessionQuery(r)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}

		events, err := service.SearchNetworkSessions(r.Context(), query)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}

		if logger != nil {
			logger.Info("served session query", "tenant_id", query.TenantID, "count", len(events))
		}
		writeAPIJSON(w, http.StatusOK, sessionsResponse{
			Count:    len(events),
			Sessions: events,
		})
	})
}

func parseSessionQuery(r *http.Request) (sessionstore.Query, error) {
	values := r.URL.Query()
	from, err := parseTimeValue(values.Get("from"))
	if err != nil {
		return sessionstore.Query{}, fmt.Errorf("invalid from: %w", err)
	}
	to, err := parseTimeValue(values.Get("to"))
	if err != nil {
		return sessionstore.Query{}, fmt.Errorf("invalid to: %w", err)
	}
	if from != nil && to != nil && from.After(*to) {
		return sessionstore.Query{}, fmt.Errorf("from must be before or equal to to")
	}

	limit, err := parsePositiveInt(values.Get("limit"))
	if err != nil {
		return sessionstore.Query{}, fmt.Errorf("invalid limit: %w", err)
	}

	query := sessionstore.Query{
		TenantID:        strings.TrimSpace(values.Get("tenant_id")),
		SubscriberID:    strings.TrimSpace(values.Get("subscriber_id")),
		SessionID:       strings.TrimSpace(values.Get("session_id")),
		Status:          strings.TrimSpace(values.Get("status")),
		NASIPAddress:    strings.TrimSpace(values.Get("nas_ip_address")),
		ClientIPAddress: strings.TrimSpace(values.Get("client_ip_address")),
		From:            from,
		To:              to,
		Limit:           limit,
	}
	if query.TenantID == "" {
		return sessionstore.Query{}, fmt.Errorf("tenant_id is required")
	}
	return query, nil
}

func parsePositiveInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("must be a positive integer")
	}
	return parsed, nil
}
