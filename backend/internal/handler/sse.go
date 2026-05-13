package handler

import (
	"fmt"
	"net/http"

	"github.com/docinsight/backend/internal/events"
	"github.com/google/uuid"
)

type SSEHandler struct {
	broker *events.Broker
}

func NewSSEHandler(broker *events.Broker) *SSEHandler {
	return &SSEHandler{broker: broker}
}

func (h *SSEHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming not supported")
		return
	}

	clientID := uuid.New().String()
	ch := h.broker.Subscribe(clientID)
	defer h.broker.Unsubscribe(clientID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()

	// Send initial connection event
	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"data\":{}}\n\n")
	flusher.Flush()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			msg, err := events.FormatSSE(event)
			if err != nil {
				continue
			}
			fmt.Fprint(w, msg)
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}
