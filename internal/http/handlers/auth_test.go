package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/auth"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
)

func TestAuthHandlerLogsInAndProtectsAdminRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := store.Open("file:auth-handler?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repository := auth.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	service := auth.NewService(repository)
	handler := NewAuthHandler(service)
	router := gin.New()
	router.POST("/api/auth/login", handler.Login)
	admin := router.Group("/api/admin")
	admin.Use(handler.RequireSession)
	admin.GET("/probe", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/admin/probe", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated admin status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin123"})
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body=%s", login.Code, login.Body.String())
	}
	result := login.Result()
	var cookie *http.Cookie
	for _, item := range result.Cookies() {
		if item.Name == auth.SessionCookieName {
			cookie = item
			break
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected HttpOnly SameSite=Lax cookie, got %#v", cookie)
	}

	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/probe", nil)
	request.AddCookie(cookie)
	router.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authenticated admin status = %d, body=%s", authorized.Code, authorized.Body.String())
	}

	if _, err := service.Session(context.Background(), cookie.Value); err != nil {
		t.Fatalf("session cookie is not usable: %v", err)
	}
}
