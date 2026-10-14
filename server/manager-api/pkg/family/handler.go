// Package family 是家庭档案的维护：人物、关系、地点。
package family

import (
	"common/middleware/httpx"
	"service/family"

	"github.com/gin-gonic/gin"
)

type Handler struct{ family *family.Service }

func New(service *family.Service) *Handler { return &Handler{family: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/persons", h.listPersons)
	group.GET("/persons/:key", h.getPerson)
	group.POST("/persons", h.savePerson)
	group.PUT("/persons/:key", h.savePerson)
	group.DELETE("/persons/:key", h.archivePerson)

	group.GET("/relations", h.listRelations)
	group.POST("/relations", h.saveRelation)
	group.PUT("/relations/:key", h.saveRelation)
	group.DELETE("/relations/:key", h.deleteRelation)

	group.GET("/places", h.listPlaces)
	group.POST("/places", h.savePlace)
	group.PUT("/places/:key", h.savePlace)
	group.DELETE("/places/:key", h.deletePlace)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) listPersons(c *gin.Context) {
	result, err := h.family.ListPersons(c.Request.Context(), familyKey(c))
	httpx.JSON(c, result, err)
}

func (h *Handler) getPerson(c *gin.Context) {
	result, err := h.family.GetPerson(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func (h *Handler) savePerson(c *gin.Context) {
	var body family.Person
	if !httpx.Bind(c, &body) {
		return
	}
	body.PersonKey = c.Param("key")
	result, err := h.family.SavePerson(c.Request.Context(), familyKey(c), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) archivePerson(c *gin.Context) {
	httpx.JSON(c, nil, h.family.ArchivePerson(c.Request.Context(), familyKey(c), c.Param("key")))
}

func (h *Handler) listRelations(c *gin.Context) {
	result, err := h.family.ListRelations(c.Request.Context(), familyKey(c), c.Query("person"))
	httpx.JSON(c, result, err)
}

func (h *Handler) saveRelation(c *gin.Context) {
	var body family.Relation
	if !httpx.Bind(c, &body) {
		return
	}
	body.RelationKey = c.Param("key")
	result, err := h.family.SaveRelation(c.Request.Context(), familyKey(c), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) deleteRelation(c *gin.Context) {
	httpx.JSON(c, nil, h.family.DeleteRelation(c.Request.Context(), familyKey(c), c.Param("key")))
}

func (h *Handler) listPlaces(c *gin.Context) {
	result, err := h.family.ListPlaces(c.Request.Context(), familyKey(c))
	httpx.JSON(c, result, err)
}

func (h *Handler) savePlace(c *gin.Context) {
	var body family.Place
	if !httpx.Bind(c, &body) {
		return
	}
	body.PlaceKey = c.Param("key")
	result, err := h.family.SavePlace(c.Request.Context(), familyKey(c), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) deletePlace(c *gin.Context) {
	httpx.JSON(c, nil, h.family.DeletePlace(c.Request.Context(), familyKey(c), c.Param("key")))
}
