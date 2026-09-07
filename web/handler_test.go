package adminweb

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerServesIndexAndSPAFallbackWithoutCaching(t *testing.T) {
	handler := NewHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html>control room</html>")},
	})

	for _, requestPath := range []string{"/admin", "/admin/", "/admin/tasks"} {
		t.Run(requestPath, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if !strings.Contains(response.Body.String(), "control room") {
				t.Fatalf("body = %q, want embedded index", response.Body.String())
			}
			if got := response.Header().Get("Cache-Control"); got != "no-cache" {
				t.Fatalf("Cache-Control = %q, want no-cache", got)
			}
		})
	}
}

func TestHandlerServesHashedAssetsWithLongCache(t *testing.T) {
	handler := NewHandler(fstest.MapFS{
		"index.html":        {Data: []byte("index")},
		"assets/app-123.js": {Data: []byte("console.log('ok')")},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/assets/app-123.js", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("Content-Type = %q, want javascript", got)
	}
}

func TestHandlerRejectsTraversalAndNonAdminPaths(t *testing.T) {
	handler := NewHandler(fstest.MapFS{"index.html": {Data: []byte("index")}})
	tests := []struct {
		path string
		want int
	}{
		{path: "/admin/%2e%2e/secrets", want: http.StatusBadRequest},
		{path: "/api/admin/tasks", want: http.StatusNotFound},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.want {
			t.Fatalf("%s status = %d, want %d", test.path, response.Code, test.want)
		}
	}
}

func TestHandlerReportsUnavailableWhenIndexIsMissing(t *testing.T) {
	handler := NewHandler(fstest.MapFS{".gitkeep": {Data: nil, Mode: fs.ModePerm}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), "admin UI not built") {
		t.Fatalf("body = %q", response.Body.String())
	}
}
