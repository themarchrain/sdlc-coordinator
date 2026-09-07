package runner

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
)

func NewHTTPHandler(service *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/providers", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": service.ProviderNames()})
	})
	mux.HandleFunc("POST /api/v1/executions", func(w http.ResponseWriter, r *http.Request) {
		var request CreateExecutionRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err)
			return
		}
		execution, err := service.Create(r.Context(), request)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, execution)
	})
	mux.HandleFunc("GET /api/v1/executions/{execution_id}", func(w http.ResponseWriter, r *http.Request) {
		execution, err := service.Get(r.PathValue("execution_id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, execution)
	})
	mux.HandleFunc("GET /api/v1/executions/{execution_id}/events", func(w http.ResponseWriter, r *http.Request) {
		after, err := strconv.ParseInt(r.URL.Query().Get("after_seq"), 10, 64)
		if r.URL.Query().Get("after_seq") == "" {
			after = 0
			err = nil
		}
		if err != nil || after < 0 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_after_seq", errors.New("after_seq must be a non-negative integer"))
			return
		}
		events, err := service.Events(r.PathValue("execution_id"), after)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": events})
	})
	mux.HandleFunc("GET /api/v1/executions/{execution_id}/result", func(w http.ResponseWriter, r *http.Request) {
		result, err := service.Result(r.PathValue("execution_id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/v1/executions/{execution_id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		execution, err := service.Cancel(r.PathValue("execution_id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, execution)
	})
	mux.HandleFunc("DELETE /api/v1/executions/{execution_id}", func(w http.ResponseWriter, r *http.Request) {
		if err := service.Delete(r.PathValue("execution_id")); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func (s *Service) ProviderNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]string, 0, len(s.providers))
	for name := range s.providers {
		items = append(items, name)
	}
	sort.Strings(items)
	return items
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err)
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, "request_conflict", err)
	case errors.Is(err, ErrNotPurgeable):
		writeError(w, http.StatusConflict, "not_purgeable", err)
	case errors.Is(err, ErrNotReady):
		writeError(w, http.StatusConflict, "result_not_ready", err)
	default:
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", err)
	}
}

func writeError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": err.Error()}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
