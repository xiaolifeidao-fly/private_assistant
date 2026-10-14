// Package skills 是做事手册：每个场景怎么查、怎么想、怎么说，模型按需读取。
package skills

import (
	"errors"

	"common/middleware/httpx"
	"service/skill"

	"github.com/gin-gonic/gin"
)

type Handler struct{ skills *skill.Service }

func New(service *skill.Service) *Handler { return &Handler{skills: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/skills", h.list)
	group.PUT("/skills/:key", h.save)
	group.GET("/skills/:key/default", h.defaultContent)
	group.POST("/skills/:key/suggest", h.suggest)
	group.GET("/skill-examples", h.listExamples)
	group.POST("/skill-examples", h.saveExample)
	group.PUT("/skill-examples/:key", h.saveExample)
	group.POST("/skill-examples/:key/status", h.exampleStatus)
}

func (h *Handler) listExamples(c *gin.Context) {
	result, err := h.skills.ListExamples(c.Request.Context(), familyKey(c), c.Query("skillKey"), c.Query("status"))
	httpx.JSON(c, result, err)
}

func (h *Handler) saveExample(c *gin.Context) {
	var body skill.Example
	if !httpx.Bind(c, &body) {
		return
	}
	body.ExampleKey = c.Param("key")
	result, err := h.skills.SaveExample(c.Request.Context(), familyKey(c), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) exampleStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	httpx.JSON(c, nil, h.skills.SetExampleStatus(c.Request.Context(), familyKey(c), c.Param("key"), body.Status))
}

// suggest 按家里的记忆给出改写建议，不保存。
func (h *Handler) suggest(c *gin.Context) {
	result, err := h.skills.Suggest(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) list(c *gin.Context) {
	result, err := h.skills.List(c.Request.Context(), familyKey(c))
	httpx.JSON(c, result, err)
}

func (h *Handler) save(c *gin.Context) {
	var body skill.Skill
	if !httpx.Bind(c, &body) {
		return
	}
	body.SkillKey = c.Param("key")
	result, err := h.skills.Save(c.Request.Context(), familyKey(c), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) defaultContent(c *gin.Context) {
	result, ok := skill.Default(c.Param("key"))
	if !ok {
		httpx.JSON(c, nil, errors.New("这本手册没有出厂内容"))
		return
	}
	httpx.JSON(c, result, nil)
}
