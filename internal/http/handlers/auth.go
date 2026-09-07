package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"sdlc-coordinator/internal/auth"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct{ service *auth.Service }

type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewAuthHandler(service *auth.Service) *AuthHandler { return &AuthHandler{service: service} }

func (handler *AuthHandler) Login(c *gin.Context) {
	var body loginBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeAdminError(c, http.StatusBadRequest, "invalid_login_request", "请输入账号和密码")
		return
	}
	token, user, err := handler.service.Login(c.Request.Context(), body.Username, body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeAdminError(c, http.StatusUnauthorized, "invalid_credentials", "账号或密码错误")
			return
		}
		writeAdminError(c, http.StatusInternalServerError, "internal_error", "登录服务暂不可用")
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, token, int(auth.DefaultSessionTTL.Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"username": user.Username})
}

func (handler *AuthHandler) Session(c *gin.Context) {
	session, err := handler.session(c)
	if err != nil {
		writeUnauthenticated(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"username": session.Username, "expires_at": session.ExpiresAt.UTC().Format(time.RFC3339)})
}

func (handler *AuthHandler) RequireSession(c *gin.Context) {
	if _, err := handler.session(c); err != nil {
		writeUnauthenticated(c)
		c.Abort()
		return
	}
	c.Next()
}

func (handler *AuthHandler) session(c *gin.Context) (*auth.AdminSession, error) {
	cookie, err := c.Request.Cookie(auth.SessionCookieName)
	if err != nil {
		return nil, auth.ErrUnauthenticated
	}
	return handler.service.Session(c.Request.Context(), strings.TrimSpace(cookie.Value))
}

func writeUnauthenticated(c *gin.Context) {
	writeAdminError(c, http.StatusUnauthorized, "unauthenticated", "请先登录管理台")
}
