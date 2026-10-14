// Package bots 是机器人配置：提示词、模型路由、工具、记忆范围。
package bots

import (
	"errors"

	"common/middleware/httpx"
	"service/bot"

	"github.com/gin-gonic/gin"
)

type Handler struct{ bots *bot.Service }

func New(service *bot.Service) *Handler { return &Handler{bots: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/bots", h.list)
	group.GET("/bots/:key", h.get)
	group.PUT("/bots/:key", h.update)
	group.GET("/bots/:key/default-prompt", h.defaultPrompt)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) list(c *gin.Context) {
	result, err := h.bots.List(c.Request.Context(), familyKey(c), false)
	httpx.JSON(c, result, err)
}

func (h *Handler) get(c *gin.Context) {
	result, err := h.bots.Get(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func (h *Handler) update(c *gin.Context) {
	var body bot.Bot
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.bots.Update(c.Request.Context(), familyKey(c), c.Param("key"), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) defaultPrompt(c *gin.Context) {
	prompt, ok := bot.DefaultPrompt(c.Param("key"))
	if !ok {
		httpx.JSON(c, nil, errors.New("这个机器人没有出厂提示词"))
		return
	}
	httpx.JSON(c, gin.H{"systemPrompt": prompt}, nil)
}
