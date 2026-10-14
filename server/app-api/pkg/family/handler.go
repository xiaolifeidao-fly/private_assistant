// Package family 是 App 端只读的家庭档案：人物卡片、关系、地点。编辑在管理端做。
package family

import (
	"time"

	"common/middleware/httpx"
	"service/family"
	"service/persona"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	family  *family.Service
	persona *persona.Service
}

func New(familyService *family.Service, personaService *persona.Service) *Handler {
	return &Handler{family: familyService, persona: personaService}
}

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/persons", h.listPersons)
	group.GET("/persons/:key", h.getPerson)
	group.GET("/persons/:key/cards", h.cardHistory)
	group.GET("/persona-changes", h.recentChanges)
	group.GET("/places", h.listPlaces)
}

// cardHistory 是某人人设卡的全部版本：App 人物页「TA 的变化」时间线。
func (h *Handler) cardHistory(c *gin.Context) {
	result, err := h.persona.History(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

// recentChanges 是最近几天人设卡的更新（默认 7 天）：App「今天」页。
func (h *Handler) recentChanges(c *gin.Context) {
	days := httpx.QueryInt(c, "days", 7)
	if days < 1 || days > 90 {
		days = 7
	}
	result, err := h.persona.Recent(c.Request.Context(), familyKey(c), time.Now().AddDate(0, 0, -days), 20)
	httpx.JSON(c, result, err)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) listPersons(c *gin.Context) {
	result, err := h.family.ListPersons(c.Request.Context(), familyKey(c))
	httpx.JSON(c, result, err)
}

func (h *Handler) getPerson(c *gin.Context) {
	ctx := c.Request.Context()
	person, err := h.family.GetPerson(ctx, familyKey(c), c.Param("key"))
	if err != nil {
		httpx.JSON(c, nil, err)
		return
	}
	relations, err := h.family.ListRelations(ctx, familyKey(c), person.PersonKey)
	if err != nil {
		httpx.JSON(c, nil, err)
		return
	}
	var card *persona.Card
	if active, ok, err := h.persona.Active(ctx, familyKey(c), person.PersonKey); err == nil && ok {
		card = &active
	}
	httpx.JSON(c, gin.H{"person": person, "relations": relations, "card": card}, nil)
}

func (h *Handler) listPlaces(c *gin.Context) {
	result, err := h.family.ListPlaces(c.Request.Context(), familyKey(c))
	httpx.JSON(c, result, err)
}
