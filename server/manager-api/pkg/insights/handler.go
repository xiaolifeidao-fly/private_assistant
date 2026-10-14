// Package insights 是每周整理：周洞察、疑似重复记忆的合并建议、执行记录。
package insights

import (
	"common/middleware/httpx"
	"service/insight"

	"github.com/gin-gonic/gin"
)

type Handler struct{ insight *insight.Service }

func New(service *insight.Service) *Handler { return &Handler{insight: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/insight/runs", h.runs)
	group.POST("/insight/weekly", h.weekly)
	group.GET("/insight/merges", h.merges)
	group.POST("/insight/merges/find", h.find)
	group.POST("/insight/merges/:key", h.decide)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) runs(c *gin.Context) {
	result, err := h.insight.Runs(c.Request.Context(), familyKey(c), 30)
	httpx.JSON(c, result, err)
}

// weekly 手动生成一次本周洞察（不受每周一次的限制）。
func (h *Handler) weekly(c *gin.Context) {
	result, err := h.insight.WeeklyInsight(c.Request.Context(), familyKey(c), true)
	httpx.JSON(c, result, err)
}

func (h *Handler) merges(c *gin.Context) {
	result, err := h.insight.ListMerges(c.Request.Context(), familyKey(c), c.DefaultQuery("status", "pending"))
	httpx.JSON(c, result, err)
}

func (h *Handler) find(c *gin.Context) {
	added, err := h.insight.FindDuplicates(c.Request.Context(), familyKey(c), true)
	httpx.JSON(c, gin.H{"added": added}, err)
}

// decide：keep 填要保留的记忆编号（另一条归档），留空表示不是重复。
func (h *Handler) decide(c *gin.Context) {
	var body struct {
		Keep string `json:"keep"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	httpx.JSON(c, nil, h.insight.DecideMerge(c.Request.Context(), familyKey(c), c.Param("key"), body.Keep))
}
