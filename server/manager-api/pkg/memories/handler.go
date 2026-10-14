// Package memories 是记忆管理：列表、增改、确认/驳回、替代、演化历史、检索调试。
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
	group.GET("/memory-types", h.types)
	group.GET("/memories", h.list)
	group.GET("/memories/:key", h.get)
	group.GET("/memories/:key/history", h.history)
	group.POST("/memories", h.create)
	group.PUT("/memories/:key", h.update)
	group.POST("/memories/:key/status", h.setStatus)
	group.POST("/memories/:key/supersede", h.supersede)
	group.POST("/memories/search", h.search)
	group.POST("/memories/reindex", h.reindex)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) types(c *gin.Context) { httpx.JSON(c, memory.Types, nil) }

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
	}
	items, total, err := h.memory.List(c.Request.Context(), familyKey(c), filter)
	httpx.JSON(c, gin.H{"items": items, "total": total}, err)
}

func (h *Handler) get(c *gin.Context) {
	result, err := h.memory.Get(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func (h *Handler) history(c *gin.Context) {
	result, err := h.memory.History(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func (h *Handler) create(c *gin.Context) {
	var body memory.Input
	if !httpx.Bind(c, &body) {
		return
	}
	body.CreatedBy = "manual"
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

func (h *Handler) setStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	httpx.JSON(c, nil, h.memory.SetStatus(c.Request.Context(), familyKey(c), c.Param("key"), body.Status))
}

func (h *Handler) supersede(c *gin.Context) {
	var body memory.Input
	if !httpx.Bind(c, &body) {
		return
	}
	body.CreatedBy = "manual"
	result, err := h.memory.Supersede(c.Request.Context(), familyKey(c), c.Param("key"), body)
	httpx.JSON(c, result, err)
}

// search 是检索调试：看某个问题会召回哪些记忆、各自得分多少。
func (h *Handler) search(c *gin.Context) {
	var body struct {
		Text           string   `json:"text"`
		Persons        []string `json:"persons"`
		Types          []string `json:"types"`
		IncludeHistory bool     `json:"includeHistory"`
		VisibleTo      string   `json:"visibleTo"`
		Limit          int      `json:"limit"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.memory.Search(c.Request.Context(), familyKey(c), memory.SearchQuery{
		Text: body.Text, Persons: body.Persons, Types: body.Types, IncludeHistory: body.IncludeHistory,
		VisibleTo: body.VisibleTo, Limit: body.Limit,
	})
	httpx.JSON(c, result, err)
}

func (h *Handler) reindex(c *gin.Context) {
	count, err := h.memory.ReindexMissing(c.Request.Context(), familyKey(c))
	httpx.JSON(c, gin.H{"count": count}, err)
}
