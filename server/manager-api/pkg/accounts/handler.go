// Package accounts 是两套账号（管理端 / App）的管理，仅管理员可用。
package accounts

import (
	"common/middleware/httpx"
	"service/identity"

	"github.com/gin-gonic/gin"
)

type Handler struct{ identity *identity.Service }

func New(service *identity.Service) *Handler { return &Handler{identity: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	admin := group.Group("/accounts", httpx.RequireAdmin())
	admin.GET("", h.list)
	admin.POST("", h.create)
	admin.PUT("/:key", h.update)
}

func (h *Handler) list(c *gin.Context) {
	result, err := h.identity.ListAccounts(c.Request.Context(), c.Query("realm"))
	httpx.JSON(c, result, err)
}

func (h *Handler) create(c *gin.Context) {
	var body identity.AccountInput
	if !httpx.Bind(c, &body) {
		return
	}
	body.FamilyKey = httpx.MustUser(c).FamilyKey
	result, err := h.identity.CreateAccount(c.Request.Context(), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) update(c *gin.Context) {
	var body identity.AccountInput
	if !httpx.Bind(c, &body) {
		return
	}
	if c.Param("key") == httpx.MustUser(c).AccountKey && body.Status != "" && body.Status != "active" {
		httpx.Fail(c, "不能停用自己")
		return
	}
	result, err := h.identity.UpdateAccount(c.Request.Context(), c.Param("key"), body)
	httpx.JSON(c, result, err)
}
