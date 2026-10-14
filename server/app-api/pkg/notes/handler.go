// Package notes 是原始记录：粘贴的聊天、截图、长段速记，提交后由后台自动整理成记忆。
package notes

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"common/middleware/httpx"
	"service/intake"

	"github.com/gin-gonic/gin"
)

const maxUpload = 50 << 20

type Handler struct {
	intake  *intake.Service
	channel string // 未指定来源时的默认值
}

func New(service *intake.Service, defaultChannel string) *Handler {
	return &Handler{intake: service, channel: defaultChannel}
}

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/notes", h.list)
	group.POST("/notes", h.create)
	group.GET("/notes/:key", h.get)
	group.POST("/notes/:key/retry", h.retry)
	group.GET("/notes/:key/attachments/:index", h.attachment)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) list(c *gin.Context) {
	items, total, err := h.intake.ListNotes(c.Request.Context(), familyKey(c), c.Query("status"),
		httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "size", 20))
	httpx.JSON(c, gin.H{"items": items, "total": total}, err)
}

func (h *Handler) get(c *gin.Context) {
	result, err := h.intake.GetNote(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func (h *Handler) retry(c *gin.Context) {
	httpx.JSON(c, nil, h.intake.Retry(c.Request.Context(), familyKey(c), c.Param("key")))
}

// create 收 multipart：text、channel、aboutPersons（逗号分隔）、occurredAt（RFC3339）、images（多个文件）、inline。
func (h *Handler) create(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUpload)
	if err := c.Request.ParseMultipartForm(maxUpload); err != nil {
		httpx.Fail(c, "上传内容太大或格式不对")
		return
	}
	input := intake.NoteInput{
		Channel:   strings.TrimSpace(c.PostForm("channel")),
		Text:      c.PostForm("text"),
		CreatedBy: httpx.MustUser(c).Realm,
		// inline=1：对话里带的照片，接下来发消息时当场整理
		Inline: c.PostForm("inline") == "1",
	}
	if input.Channel == "" {
		input.Channel = h.channel
	}
	for _, key := range strings.Split(c.PostForm("aboutPersons"), ",") {
		if key = strings.TrimSpace(key); key != "" {
			input.AboutPersons = append(input.AboutPersons, key)
		}
	}
	if value := strings.TrimSpace(c.PostForm("occurredAt")); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			httpx.Fail(c, "occurredAt 格式应为 RFC3339")
			return
		}
		local := parsed.Local()
		input.OccurredAt = &local
	}
	if c.Request.MultipartForm != nil {
		for index, header := range c.Request.MultipartForm.File["images"] {
			file, err := header.Open()
			if err != nil {
				httpx.Fail(c, fmt.Sprintf("读取第 %d 张图片失败", index+1))
				return
			}
			data, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				httpx.Fail(c, fmt.Sprintf("读取第 %d 张图片失败", index+1))
				return
			}
			// 以内容嗅探为准，不信任客户端声明的类型
			input.Images = append(input.Images, intake.Image{MediaType: http.DetectContentType(data), Data: data})
		}
	}
	result, err := h.intake.CreateNote(c.Request.Context(), familyKey(c), input)
	httpx.JSON(c, result, err)
}

func (h *Handler) attachment(c *gin.Context) {
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		httpx.Fail(c, "序号不对")
		return
	}
	image, err := h.intake.Attachment(c.Request.Context(), familyKey(c), c.Param("key"), index)
	if err != nil {
		if errors.Is(err, intake.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		httpx.Fail(c, err.Error())
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.Data(http.StatusOK, image.MediaType, image.Data)
}
