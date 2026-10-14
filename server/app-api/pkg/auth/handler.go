// Package auth 是 App 端的登录、登出、当前用户、改密码（与管理端同形，令牌按 realm 隔离）。
package auth

import (
	"common/middleware/httpx"
	"service/identity"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	identity *identity.Service
	realm    string
}

func New(service *identity.Service, realm string) *Handler {
	return &Handler{identity: service, realm: realm}
}

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.POST("/auth/login", h.login)
	group.POST("/auth/logout", httpx.RequireUser(), h.logout)
	group.GET("/auth/me", httpx.RequireUser(), h.me)
	group.POST("/auth/password", httpx.RequireUser(), h.password)
}

func (h *Handler) login(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.identity.Login(c.Request.Context(), identity.LoginInput{
		Realm: h.realm, Username: body.Username, Password: body.Password,
		ClientIP: httpx.ClientIP(c), UserAgent: c.GetHeader("User-Agent"),
	})
	httpx.JSON(c, result, err)
}

func (h *Handler) logout(c *gin.Context) {
	httpx.JSON(c, nil, h.identity.Logout(c.Request.Context(), httpx.Token(c)))
}

func (h *Handler) me(c *gin.Context) {
	httpx.JSON(c, httpx.MustUser(c), nil)
}

func (h *Handler) password(c *gin.Context) {
	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	httpx.JSON(c, nil, h.identity.ChangePassword(c.Request.Context(), httpx.MustUser(c).AccountKey, body.OldPassword, body.NewPassword))
}
