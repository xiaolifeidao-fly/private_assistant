// Package diet 是管理端看、改孩子的饮食：计划和实际按周对照，能手动改，也能说一句话交给模型整理（service/diet）。
// 接口和 App 端（app-api/pkg/diet）一致；管理端账号是我自己（admin / member），能改计划。
package diet

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"common/middleware/httpx"
	"service/diet"

	"github.com/gin-gonic/gin"
)

const maxUpload = 25 << 20

type Handler struct{ diet *diet.Service }

func New(service *diet.Service) *Handler { return &Handler{diet: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/diet", h.overview)
	group.PUT("/diet/plans", h.savePlan)
	group.POST("/diet/plans/copy", h.copyPlans)
	group.POST("/diet/plans/capture", h.capturePlan)
	group.POST("/diet/records", h.createRecord)
	group.POST("/diet/records/capture", h.captureRecord)
	group.PUT("/diet/records/:key", h.updateRecord)
	group.DELETE("/diet/records/:key", h.deleteRecord)
	group.GET("/diet/records/:key/photos/:index", h.photo)
	group.GET("/diet/review", h.review)
	group.POST("/diet/review", h.createReview)
}

func actor(c *gin.Context) (string, diet.Actor) {
	user := httpx.MustUser(c)
	return user.FamilyKey, diet.Actor{Account: user.AccountKey, Role: user.Role}
}

// overview ?child=&from=YYYY-MM-DD&days=7
func (h *Handler) overview(c *gin.Context) {
	family, who := actor(c)
	result, err := h.diet.Overview(c.Request.Context(), family, c.Query("child"), c.Query("from"), httpx.QueryInt(c, "days", 1), who)
	httpx.JSON(c, result, err)
}

func (h *Handler) savePlan(c *gin.Context) {
	var body diet.PlanInput
	if !httpx.Bind(c, &body) {
		return
	}
	family, who := actor(c)
	result, err := h.diet.SavePlan(c.Request.Context(), family, body, who)
	httpx.JSON(c, gin.H{"plan": result}, err)
}

func (h *Handler) copyPlans(c *gin.Context) {
	var body diet.CopyInput
	if !httpx.Bind(c, &body) {
		return
	}
	family, who := actor(c)
	count, err := h.diet.CopyPlans(c.Request.Context(), family, body, who)
	httpx.JSON(c, gin.H{"count": count}, err)
}

// capturePlan 收 multipart：text、images、child、from、days。
func (h *Handler) capturePlan(c *gin.Context) {
	images, ok := readImages(c)
	if !ok {
		return
	}
	days, _ := strconv.Atoi(c.PostForm("days"))
	family, who := actor(c)
	result, err := h.diet.CapturePlan(c.Request.Context(), family, diet.PlanCaptureInput{
		ChildKey: c.PostForm("child"), Text: c.PostForm("text"), Images: images, From: c.PostForm("from"), Days: days,
	}, who)
	httpx.JSON(c, result, err)
}

func (h *Handler) createRecord(c *gin.Context) {
	var body diet.RecordInput
	if !httpx.Bind(c, &body) {
		return
	}
	family, who := actor(c)
	result, err := h.diet.CreateRecord(c.Request.Context(), family, body, who)
	httpx.JSON(c, result, err)
}

// captureRecord 收 multipart：text、images、child、at（RFC3339，拍照/说话的时间）。
func (h *Handler) captureRecord(c *gin.Context) {
	images, ok := readImages(c)
	if !ok {
		return
	}
	input := diet.CaptureInput{ChildKey: c.PostForm("child"), Text: c.PostForm("text"), Images: images}
	if value := strings.TrimSpace(c.PostForm("at")); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			httpx.Fail(c, "at 格式应为 RFC3339")
			return
		}
		input.At = &parsed
	}
	family, who := actor(c)
	result, err := h.diet.Capture(c.Request.Context(), family, input, who)
	httpx.JSON(c, result, err)
}

func (h *Handler) updateRecord(c *gin.Context) {
	var body diet.RecordInput
	if !httpx.Bind(c, &body) {
		return
	}
	family, who := actor(c)
	result, err := h.diet.UpdateRecord(c.Request.Context(), family, c.Param("key"), body, who)
	httpx.JSON(c, result, err)
}

func (h *Handler) deleteRecord(c *gin.Context) {
	family, who := actor(c)
	httpx.JSON(c, gin.H{"ok": true}, h.diet.DeleteRecord(c.Request.Context(), family, c.Param("key"), who))
}

// review ?child=&date=YYYY-MM-DD：那天所在那周的计划对照点评，没有时 review 为 null。
func (h *Handler) review(c *gin.Context) {
	family, _ := actor(c)
	result, err := h.diet.GetReview(c.Request.Context(), family, c.Query("child"), c.Query("date"))
	httpx.JSON(c, gin.H{"review": result}, err)
}

// createReview 现在就生成（覆盖）那周的点评。爸爸妈妈能用。
func (h *Handler) createReview(c *gin.Context) {
	var body struct {
		ChildKey string `json:"childKey"`
		Date     string `json:"date"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	family, who := actor(c)
	result, err := h.diet.CreateReview(c.Request.Context(), family, body.ChildKey, body.Date, who)
	httpx.JSON(c, result, err)
}

func (h *Handler) photo(c *gin.Context) {
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		httpx.Fail(c, "序号不对")
		return
	}
	family, _ := actor(c)
	mediaType, data, err := h.diet.PhotoData(c.Request.Context(), family, c.Param("key"), index)
	if err != nil {
		if errors.Is(err, diet.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		httpx.Fail(c, err.Error())
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.Data(http.StatusOK, mediaType, data)
}

// readImages 读 multipart 里的 images；以内容嗅探的类型为准。失败时已写好响应。
func readImages(c *gin.Context) ([]diet.Image, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUpload)
	if err := c.Request.ParseMultipartForm(maxUpload); err != nil {
		httpx.Fail(c, "上传内容太大或格式不对")
		return nil, false
	}
	var images []diet.Image
	if c.Request.MultipartForm == nil {
		return images, true
	}
	for index, header := range c.Request.MultipartForm.File["images"] {
		file, err := header.Open()
		if err != nil {
			httpx.Fail(c, fmt.Sprintf("读取第 %d 张照片失败", index+1))
			return nil, false
		}
		data, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			httpx.Fail(c, fmt.Sprintf("读取第 %d 张照片失败", index+1))
			return nil, false
		}
		images = append(images, diet.Image{MediaType: http.DetectContentType(data), Data: data})
	}
	return images, true
}
