// Package speech 是管理端的语音转文字（对话补充人设卡等对话框里说话）。录音不保存。
package speech

import (
	"net/http"

	"common/middleware/httpx"
	"service/llm"

	"github.com/gin-gonic/gin"
)

type Handler struct{ llm *llm.Gateway }

func New(gateway *llm.Gateway) *Handler { return &Handler{llm: gateway} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.POST("/speech/transcribe", h.transcribe)
}

// transcribe 收 multipart 的 audio 文件，返回 {"text": "..."}。
func (h *Handler) transcribe(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, llm.MaxAudioBytes+1<<20)
	header, err := c.FormFile("audio")
	if err != nil {
		httpx.Fail(c, "没收到录音，或者录音太长了")
		return
	}
	text, err := h.llm.TranscribeUpload(c.Request.Context(), header)
	httpx.JSON(c, gin.H{"text": text}, err)
}
