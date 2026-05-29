package api

import (
	"fmt"
	"log/slog"
	"net/http"

	a2atransport "github.com/asimarora/semantic-intelligence-layer/internal/transport/a2a"
)

func registerA2ARoutes(mux *http.ServeMux, logger *slog.Logger, handler *a2atransport.Handler, initErr error) {
	mux.HandleFunc("/v1/a2a/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method %s is not allowed", r.Method))
			return
		}
		if initErr != nil {
			writeAPIError(w, http.StatusNotImplemented, initErr)
			return
		}
		if handler == nil {
			writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("a2a interface is not configured"))
			return
		}
		if logger != nil {
			logger.Info("handling a2a message")
		}
		handler.ServeMessage(w, r)
	})
}
