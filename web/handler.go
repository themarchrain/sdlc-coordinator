package adminweb

import (
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
)

type spaHandler struct {
	files fs.FS
}

func NewHandler(files fs.FS) http.Handler {
	return &spaHandler{files: files}
}

func (handler *spaHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		response.Header().Set("Allow", "GET, HEAD")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestPath, err := url.PathUnescape(request.URL.EscapedPath())
	if err != nil {
		http.Error(response, "invalid admin path", http.StatusBadRequest)
		return
	}
	if requestPath != "/admin" && !strings.HasPrefix(requestPath, "/admin/") {
		http.NotFound(response, request)
		return
	}

	relativePath := strings.TrimPrefix(requestPath, "/admin")
	for _, segment := range strings.Split(relativePath, "/") {
		if segment == ".." {
			http.Error(response, "invalid admin path", http.StatusBadRequest)
			return
		}
	}
	relativePath = strings.TrimPrefix(path.Clean("/"+relativePath), "/")

	if relativePath != "" && relativePath != "." {
		if contents, readErr := fs.ReadFile(handler.files, relativePath); readErr == nil {
			handler.writeFile(response, request, relativePath, contents)
			return
		}
		if path.Ext(relativePath) != "" {
			http.NotFound(response, request)
			return
		}
	}

	index, err := fs.ReadFile(handler.files, "index.html")
	if err != nil {
		http.Error(response, "admin UI not built", http.StatusServiceUnavailable)
		return
	}
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	response.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		_, _ = response.Write(index)
	}
}

func (handler *spaHandler) writeFile(response http.ResponseWriter, request *http.Request, filename string, contents []byte) {
	if filename == "index.html" {
		response.Header().Set("Cache-Control", "no-cache")
	} else {
		response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if contentType := mime.TypeByExtension(path.Ext(filename)); contentType != "" {
		response.Header().Set("Content-Type", contentType)
	}
	response.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		_, _ = response.Write(contents)
	}
}
