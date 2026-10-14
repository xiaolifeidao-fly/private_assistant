// Package models 是模型网关的配置：提供商、任务路由、调用日志。
package models

import (
	"common/middleware/httpx"
	"service/llm"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	llm *llm.Gateway
	// embeddingChanged 在 embedding 路由保存后调用：换了向量模型要给全部记忆重算向量。
	embeddingChanged func()
}

func New(gateway *llm.Gateway, embeddingChanged func()) *Handler {
	return &Handler{llm: gateway, embeddingChanged: embeddingChanged}
}

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/llm/providers", h.listProviders)
	admin := group.Group("/llm", httpx.RequireAdmin())
	admin.POST("/providers", h.saveProvider)
	admin.DELETE("/providers/:key", h.deleteProvider)
	group.GET("/llm/routes", h.listRoutes)
	admin.PUT("/routes/:task", h.saveRoute)
	admin.POST("/routes/:task/test", h.testRoute)
	group.GET("/llm/calls", h.listCalls)
}

func (h *Handler) listProviders(c *gin.Context) {
	result, err := h.llm.ListProviders(c.Request.Context())
	httpx.JSON(c, result, err)
}

func (h *Handler) saveProvider(c *gin.Context) {
	var body llm.Provider
	if !httpx.Bind(c, &body) {
		return
	}
	httpx.JSON(c, nil, h.llm.SaveProvider(c.Request.Context(), body))
}

func (h *Handler) deleteProvider(c *gin.Context) {
	httpx.JSON(c, nil, h.llm.DeleteProvider(c.Request.Context(), c.Param("key")))
}

func (h *Handler) listRoutes(c *gin.Context) {
	result, err := h.llm.ListRoutes(c.Request.Context())
	httpx.JSON(c, result, err)
}

func (h *Handler) saveRoute(c *gin.Context) {
	var body llm.RouteConfig
	if !httpx.Bind(c, &body) {
		return
	}
	body.Task = c.Param("task")
	err := h.llm.SaveRoute(c.Request.Context(), body)
	if err == nil && body.Task == llm.TaskEmbedding && h.embeddingChanged != nil {
		h.embeddingChanged()
	}
	httpx.JSON(c, nil, err)
}

func (h *Handler) testRoute(c *gin.Context) {
	result, err := h.llm.TestRoute(c.Request.Context(), c.Param("task"))
	httpx.JSON(c, gin.H{"result": result}, err)
}

func (h *Handler) listCalls(c *gin.Context) {
	items, total, err := h.llm.ListCalls(c.Request.Context(), httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "size", 50))
	httpx.JSON(c, gin.H{"items": items, "total": total}, err)
}
