// Package memories 是 App 端的记忆：速记、待确认队列、浏览。
package memories

import (
	"strings"

	"common/middleware/httpx"
	"service/memory"

	"github.com/gin-gonic/gin"
)

type Handler struct{ memory *memory.Service }

func New(service *memory.Service) *Handler { return &Handler{memory: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/memory-types", func(c *gin.Context) { httpx.JSON(c, memory.Types, nil) })
	group.GET("/memories", h.list)
	group.GET("/memories/pending-count", h.pendingCount)
	group.GET("/memories/:key", h.get)
	group.GET("/memories/:key/history", h.history)
	group.POST("/memories", h.create)
	group.PUT("/memories/:key", h.update)
	group.POST("/memories/:key/status", h.setStatus)
	group.PUT("/memories/:key/visibility", h.setVisibility)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) list(c *gin.Context) {
	filter := memory.ListFilter{
		Person: c.Query("person"), Keyword: c.Query("keyword"), Source: c.Query("source"),
		Page: httpx.QueryInt(c, "page", 1), Size: httpx.QueryInt(c, "size", 30),
	}
	if value := c.Query("types"); value != "" {
		filter.Types = strings.Split(value, ",")
	}
	if value := c.Query("statuses"); value != "" {
		filter.Statuses = strings.Split(value, ",")
	} else {
		filter.Statuses = []string{memory.StatusActive, memory.StatusPending}
	}
	items, total, err := h.memory.List(c.Request.Context(), familyKey(c), filter)
	httpx.JSON(c, gin.H{"items": items, "total": total}, err)
}

func (h *Handler) pendingCount(c *gin.Context) {
	httpx.JSON(c, gin.H{"count": h.memory.CountPending(c.Request.Context(), familyKey(c))}, nil)
}

func (h *Handler) get(c *gin.Context) {
	result, err := h.memory.Get(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func (h *Handler) history(c *gin.Context) {
	result, err := h.memory.History(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

// create 是速记：手动选好类型和人，直接生效。
func (h *Handler) create(c *gin.Context) {
	var body memory.Input
	if !httpx.Bind(c, &body) {
		return
	}
	body.CreatedBy = "manual"
	body.Status = memory.StatusActive
	result, err := h.memory.Create(c.Request.Context(), familyKey(c), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) update(c *gin.Context) {
	var body memory.Input
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.memory.Update(c.Request.Context(), familyKey(c), c.Param("key"), body)
	httpx.JSON(c, result, err)
}

// setVisibility 改谁能看到：private 只有自己 / family 全家。只有记下它的人能改。
func (h *Handler) setVisibility(c *gin.Context) {
	var body struct {
		Visibility string `json:"visibility"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.memory.SetVisibility(c.Request.Context(), familyKey(c), c.Param("key"), body.Visibility)
	httpx.JSON(c, result, err)
}

func (h *Handler) setStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	httpx.JSON(c, nil, h.memory.SetStatus(c.Request.Context(), familyKey(c), c.Param("key"), body.Status))
}
