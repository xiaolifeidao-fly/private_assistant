// Package conversations 是对话回看：管理端能看所有会话、每条消息、工具调用和冻结的系统提示词。
package conversations

import (
	"common/middleware/httpx"
	"service/chat"

	"github.com/gin-gonic/gin"
)

type Handler struct{ chat *chat.Service }

func New(service *chat.Service) *Handler { return &Handler{chat: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/conversations", h.list)
	group.GET("/conversations/:key/messages", h.messages)
	group.GET("/conversations/:key/system", h.system)
	group.GET("/messages/:key/eval-case", h.evalCase)
}

// 管理端不按账号过滤：AccountKey 留空。
func owner(c *gin.Context) chat.Owner { return chat.Owner{FamilyKey: httpx.MustUser(c).FamilyKey} }

func (h *Handler) list(c *gin.Context) {
	items, total, err := h.chat.ListConversations(c.Request.Context(), owner(c), c.Query("botKey"),
		httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "size", 30))
	httpx.JSON(c, gin.H{"items": items, "total": total}, err)
}

func (h *Handler) messages(c *gin.Context) {
	result, err := h.chat.Messages(c.Request.Context(), owner(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func (h *Handler) system(c *gin.Context) {
	result, err := h.chat.SystemSnapshot(c.Request.Context(), owner(c), c.Param("key"))
	httpx.JSON(c, gin.H{"system": result}, err)
}

func (h *Handler) evalCase(c *gin.Context) {
	result, err := h.chat.EvalCase(c.Request.Context(), owner(c), c.Param("key"))
	httpx.JSON(c, result, err)
}
