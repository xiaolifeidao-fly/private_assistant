// Package settings 是对话规则：危机词、上下文预算、自动学习的节奏，以及危机命中记录。
package settings

import (
	"encoding/json"
	"errors"
	"io"

	"common/middleware/httpx"
	"service/chat"
	"service/setting"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	settings *setting.Service
	chat     *chat.Service
}

func New(settings *setting.Service, chatService *chat.Service) *Handler {
	return &Handler{settings: settings, chat: chatService}
}

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/settings/:group", h.get)
	group.PUT("/settings/:group", h.save)
	group.DELETE("/settings/:group", h.reset)
	group.GET("/settings/:group/default", h.defaults)
	group.GET("/safety-hits", h.safetyHits)
	group.GET("/segment-rolls", h.segmentRolls)
}

func (h *Handler) segmentRolls(c *gin.Context) {
	result, err := h.chat.SegmentRolls(c.Request.Context(), chat.Owner{FamilyKey: httpx.MustUser(c).FamilyKey}, httpx.QueryInt(c, "limit", 50))
	httpx.JSON(c, result, err)
}

func (h *Handler) get(c *gin.Context) {
	result, err := h.settings.Get(c.Request.Context(), c.Param("group"))
	httpx.JSON(c, result, err)
}

func (h *Handler) save(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil || !json.Valid(body) {
		httpx.Fail(c, "格式不对")
		return
	}
	group := c.Param("group")
	if err := h.settings.Save(c.Request.Context(), group, body); err != nil {
		httpx.JSON(c, nil, err)
		return
	}
	result, err := h.settings.Get(c.Request.Context(), group)
	httpx.JSON(c, result, err)
}

func (h *Handler) reset(c *gin.Context) {
	group := c.Param("group")
	if err := h.settings.Reset(c.Request.Context(), group); err != nil {
		httpx.JSON(c, nil, err)
		return
	}
	result, err := h.settings.Get(c.Request.Context(), group)
	httpx.JSON(c, result, err)
}

func (h *Handler) defaults(c *gin.Context) {
	result, ok := setting.Defaults(c.Param("group"))
	if !ok {
		httpx.JSON(c, nil, errors.New("没有这个分组"))
		return
	}
	httpx.JSON(c, result, nil)
}

func (h *Handler) safetyHits(c *gin.Context) {
	result, err := h.chat.SafetyHits(c.Request.Context(), chat.Owner{FamilyKey: httpx.MustUser(c).FamilyKey}, httpx.QueryInt(c, "limit", 100))
	httpx.JSON(c, result, err)
}
