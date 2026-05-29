package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	channelcommon "github.com/asimarora/semantic-intelligence-layer/internal/channels/common"
)

type response struct {
	Accepted bool   `json:"accepted"`
	Channel  string `json:"channel"`
	EventID  string `json:"event_id"`
}

func Register(mux *http.ServeMux, logger *slog.Logger, registry channelcommon.Registry, requestBodyLimit int64) error {
	if mux == nil {
		return fmt.Errorf("http mux is required")
	}
	if requestBodyLimit <= 0 {
		requestBodyLimit = 1 << 20
	}

	for _, cfg := range registry.Configs {
		cfg := cfg
		path := strings.TrimSpace(cfg.WebhookPath)
		if path == "" {
			return fmt.Errorf("channel %s webhook_path is required", cfg.ID)
		}
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, fmt.Sprintf("method %s is not allowed", r.Method), http.StatusMethodNotAllowed)
				return
			}

			defer r.Body.Close()
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, requestBodyLimit))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
				return
			}

			eventID := fmt.Sprintf("%s:%d", cfg.ID, time.Now().UTC().UnixNano())
			if logger != nil {
				logger.Info("accepted channel webhook event", "channel", cfg.ID, "event_id", eventID)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(response{
				Accepted: true,
				Channel:  cfg.ID,
				EventID:  eventID,
			})
		})
	}
	return nil
}
