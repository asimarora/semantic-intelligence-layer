package api

import (
	"fmt"
	"log/slog"
	"net/http"

	chattransport "github.com/asimarora/semantic-intelligence-layer/internal/transport/chat"
)

func registerChatRoutes(mux *http.ServeMux, logger *slog.Logger, handler *chattransport.Handler, initErr error) {
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method %s is not allowed", r.Method))
			return
		}
		if initErr != nil {
			writeAPIError(w, http.StatusNotImplemented, initErr)
			return
		}
		if handler == nil {
			writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("chat interface is not configured"))
			return
		}
		if logger != nil {
			logger.Info("serving chat interface")
		}
		handler.ServePage(w, r)
	})

	mux.HandleFunc("/v1/chat/ws", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method %s is not allowed", r.Method))
			return
		}
		if initErr != nil {
			writeAPIError(w, http.StatusNotImplemented, initErr)
			return
		}
		if handler == nil {
			writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("chat interface is not configured"))
			return
		}
		handler.ServeWebSocket(w, r)
	})
}
