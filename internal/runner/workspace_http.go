package runner

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"sdlc-coordinator/internal/workspace"
	"sdlc-coordinator/internal/workspaceproto"
)

// HTTPOption 定制 Runner HTTP 面。
type HTTPOption func(*httpOptions)

type httpOptions struct {
	workspaceOps  *WorkspaceOps
	internalToken string
}

// WithWorkspaceOperations 挂载受控工作区操作面与部署级令牌：令牌为空表示
// 通道未配置（所有 /internal/* 请求 fail-closed 503），绝不静默放行。
func WithWorkspaceOperations(ops *WorkspaceOps, internalToken string) HTTPOption {
	return func(options *httpOptions) {
		options.workspaceOps = ops
		options.internalToken = strings.TrimSpace(internalToken)
	}
}

func newHTTPOptions(opts []HTTPOption) *httpOptions {
	options := &httpOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}
	return options
}

func registerWorkspaceRoutes(mux *http.ServeMux, options *httpOptions) {
	mux.HandleFunc("POST /internal/workspace/prepare", options.authorize(func(w http.ResponseWriter, r *http.Request) {
		var request workspaceproto.PrepareRequest
		if !decodeWorkspaceRequest(w, r, &request) {
			return
		}
		result, err := options.workspaceOps.PrepareWorkspace(r.Context(), request)
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc("POST /internal/workspace/bundle", options.authorize(func(w http.ResponseWriter, r *http.Request) {
		var request workspaceproto.TaskBundleRequest
		if !decodeWorkspaceRequest(w, r, &request) {
			return
		}
		result, err := options.workspaceOps.PublishTaskBundle(r.Context(), request)
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc("POST /internal/workspace/commit", options.authorize(func(w http.ResponseWriter, r *http.Request) {
		var request workspaceproto.CommitRequest
		if !decodeWorkspaceRequest(w, r, &request) {
			return
		}
		result, err := options.workspaceOps.CommitWorkspace(r.Context(), request)
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc("POST /internal/workspace/push", options.authorize(func(w http.ResponseWriter, r *http.Request) {
		var request workspaceproto.PushRequest
		if !decodeWorkspaceRequest(w, r, &request) {
			return
		}
		result, err := options.workspaceOps.PushWorkspace(r.Context(), request)
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc("POST /internal/workspace/cleanup", options.authorize(func(w http.ResponseWriter, r *http.Request) {
		var request workspaceproto.CleanupRequest
		if !decodeWorkspaceRequest(w, r, &request) {
			return
		}
		result, err := options.workspaceOps.CleanupWorkspace(r.Context(), request)
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.HandleFunc("GET /internal/operations/{operation_id}", options.authorize(func(w http.ResponseWriter, r *http.Request) {
		record, err := options.workspaceOps.GetOperation(r.PathValue("operation_id"))
		if err != nil {
			writeWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, record)
	}))
}

// authorize 是受控操作面的部署级鉴权：通道未配置一律 503（fail-closed），
// 令牌不匹配 401（常量时间比较）；业务 token / Git 凭据不参与本层。
func (o *httpOptions) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o.workspaceOps == nil || o.internalToken == "" {
			writeError(w, http.StatusServiceUnavailable, "channel_closed",
				errors.New("runner internal workspace channel is not configured"))
			return
		}
		const bearerPrefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) ||
			subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(header, bearerPrefix)), []byte(o.internalToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized",
				errors.New("runner internal operations require the deployment token"))
			return
		}
		next(w, r)
	}
}

func decodeWorkspaceRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err)
		return false
	}
	return true
}

// writeWorkspaceError 把受控操作面的错误映射为可判定的状态码：冲突/在途/
// 容量与未接纳语义都显式区分，调用方据此决定重试还是放弃。
func writeWorkspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrOperationConflict):
		writeError(w, http.StatusConflict, "operation_conflict", err)
	case errors.Is(err, ErrOperationInFlight):
		writeError(w, http.StatusConflict, "operation_in_flight", err)
	case errors.Is(err, ErrOperationNotFound):
		writeError(w, http.StatusNotFound, "operation_not_found", err)
	case errors.Is(err, ErrServiceNotAccepting):
		writeError(w, http.StatusServiceUnavailable, "not_accepting", err)
	case errors.Is(err, ErrTooManyOperations):
		writeError(w, http.StatusServiceUnavailable, "capacity_exhausted", err)
	case errors.Is(err, ErrNotAuthorized):
		writeError(w, http.StatusForbidden, "not_authorized", err)
	case errors.Is(err, workspace.ErrWorkspaceConflict):
		writeError(w, http.StatusConflict, "workspace_conflict", err)
	case errors.Is(err, workspace.ErrDeveloperDeliveryConflict):
		writeError(w, http.StatusConflict, "delivery_conflict", err)
	case errors.Is(err, workspace.ErrNoDeveloperChanges):
		writeError(w, http.StatusUnprocessableEntity, "no_changes", err)
	default:
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", err)
	}
}
