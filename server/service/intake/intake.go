// Package intake 是记忆的自动入口：原始记录（速记长文、粘贴的聊天、截图）落库后，
// 由后台 worker 抽取成候选记忆，和已有记忆比对后写入。
//
// 原始记录只增不改，是所有自动记忆的出处；抽取出错可以重跑。
// 它和 chat 一样处在依赖图上层，直接使用 family / memory / llm。
package intake

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"common/keys"
	"common/middleware/db"
	"common/viewer"
	"service/family"
	"service/llm"
	"service/memory"

	"gorm.io/gorm"
)

const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"

	maxImages     = 9
	maxImageBytes = 5 * 1024 * 1024
	maxTextRunes  = 30000
	maxAttempts   = 3
)

var ErrNotFound = errors.New("记录不存在")

// Storage 存附件，由 common/objectstore 实现（本机目录或阿里云 OSS）。
type Storage interface {
	Save(path string, data []byte) error
	Load(path string) ([]byte, error)
}

type Config struct {
	// AutoConfirm：抽取置信度不低于它、又不和已有记忆冲突的，直接生效；否则进待确认。
	AutoConfirm float64
}

type Service struct {
	db      *gorm.DB
	family  *family.Service
	memory  *memory.Service
	llm     *llm.Gateway
	storage Storage
	config  Config
	wake    chan struct{}
}

func New(database *gorm.DB, familyService *family.Service, memoryService *memory.Service, gateway *llm.Gateway, storage Storage, config Config) *Service {
	if config.AutoConfirm <= 0 || config.AutoConfirm > 1 {
		config.AutoConfirm = 0.85
	}
	return &Service{
		db: database, family: familyService, memory: memoryService, llm: gateway, storage: storage, config: config,
		wake: make(chan struct{}, 1),
	}
}

type Attachment struct {
	Path      string `json:"path"`
	MediaType string `json:"mediaType"`
	Size      int    `json:"size"`
}

// Result 是一次抽取的结果摘要。
type Result struct {
	Summary        string   `json:"summary"`
	Added          []string `json:"added"`          // 直接生效的新记忆
	Pending        []string `json:"pending"`        // 进待确认的新记忆
	Updated        []string `json:"updated"`        // 补充了细节的已有记忆
	Duplicates     []string `json:"duplicates"`     // 已经记过的
	UnknownPersons []string `json:"unknownPersons"` // 认不出来的人，提示去家庭档案里添加
}

type Note struct {
	NoteKey         string       `json:"noteKey"`
	Channel         string       `json:"channel"`
	RawText         string       `json:"rawText"`
	Attachments     []Attachment `json:"attachments"`
	AboutPersons    []string     `json:"aboutPersons"`
	OccurredAt      *time.Time   `json:"occurredAt"`
	ExtractStatus   string       `json:"extractStatus"`
	ExtractAttempts int          `json:"extractAttempts"`
	ExtractedAt     *time.Time   `json:"extractedAt"`
	Result          *Result      `json:"result"`
	ExtractError    string       `json:"extractError"`
	CreatedBy       string       `json:"createdBy"`
	OwnerAccount    string       `json:"ownerAccount"`
	Visibility      string       `json:"visibility"`
	CreatedAt       time.Time    `json:"createdAt"`
}

type Image struct {
	MediaType string
	Data      []byte
}

type NoteInput struct {
	Channel      string // quick_note / paste / screenshot / capture / manager
	Text         string
	AboutPersons []string
	OccurredAt   *time.Time
	Images       []Image
	CreatedBy    string
	// Visibility 这条记录和从它整理出的记忆谁能看到：private（默认）/ family
	Visibility string
	// Inline 表示调用方马上会用 ProcessNow 当场整理，不用唤醒后台 worker。
	Inline bool
}

var allowedImageTypes = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "image/gif": "gif"}

// CreateNote 保存原始记录（含附件）并排进抽取队列。
func (s *Service) CreateNote(ctx context.Context, familyKey string, input NoteInput) (Note, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" && len(input.Images) == 0 {
		return Note{}, errors.New("内容和截图至少要有一样")
	}
	if len([]rune(text)) > maxTextRunes {
		return Note{}, fmt.Errorf("内容太长了（超过 %d 字），请分几次提交", maxTextRunes)
	}
	if len(input.Images) > maxImages {
		return Note{}, fmt.Errorf("一次最多 %d 张截图", maxImages)
	}
	noteKey := keys.New("note")
	var attachments []Attachment
	for index, image := range input.Images {
		extension, ok := allowedImageTypes[image.MediaType]
		if !ok {
			return Note{}, fmt.Errorf("第 %d 张图片格式不支持（%s），请用 JPG/PNG", index+1, image.MediaType)
		}
		if len(image.Data) > maxImageBytes {
			return Note{}, fmt.Errorf("第 %d 张图片超过 5MB", index+1)
		}
		path := fmt.Sprintf("notes/%s/%s/%d.%s", familyKey, noteKey, index, extension)
		if err := s.storage.Save(path, image.Data); err != nil {
			return Note{}, fmt.Errorf("保存图片失败: %w", err)
		}
		attachments = append(attachments, Attachment{Path: path, MediaType: image.MediaType, Size: len(image.Data)})
	}
	channel := input.Channel
	if channel == "" {
		channel = "paste"
		if len(input.Images) > 0 {
			channel = "screenshot"
		}
	}
	row := noteRow{
		NoteKey: noteKey, FamilyKey: familyKey, Channel: channel, RawText: text,
		Attachments: db.MustJSON(attachments), AboutPersons: db.StringList(input.AboutPersons),
		OccurredAt: input.OccurredAt, ExtractStatus: StatusQueued, CreatedBy: defaultString(input.CreatedBy, "manual"),
		OwnerAccount: viewer.Owner(ctx), Visibility: viewer.Resolve(ctx, input.Visibility),
		CreatedAt: time.Now(),
	}
	if len(attachments) == 0 {
		row.Attachments = nil
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return Note{}, err
	}
	if !input.Inline {
		s.Wake()
	}
	return row.toDTO(), nil
}

func (s *Service) ListNotes(ctx context.Context, familyKey, status string, page, size int) ([]Note, int64, error) {
	query := s.db.WithContext(ctx).Model(&noteRow{}).Where("family_key = ?", familyKey)
	if status != "" {
		query = query.Where("extract_status = ?", status)
	}
	var total int64
	query.Count(&total)
	if size <= 0 || size > 100 {
		size = 20
	}
	if page <= 0 {
		page = 1
	}
	var rows []noteRow
	if err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	result := make([]Note, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, total, nil
}

func (s *Service) GetNote(ctx context.Context, familyKey, noteKey string) (Note, error) {
	row, err := s.getRow(ctx, familyKey, noteKey)
	if err != nil {
		return Note{}, err
	}
	return row.toDTO(), nil
}

// Retry 把一条失败（或已完成）的记录重新排队抽取。已经写入的记忆不会重复：
// 比对环节会把它们识别为「已经记过」。
func (s *Service) Retry(ctx context.Context, familyKey, noteKey string) error {
	row, err := s.getRow(ctx, familyKey, noteKey)
	if err != nil {
		return err
	}
	if row.ExtractStatus == StatusRunning || row.ExtractStatus == StatusQueued {
		return errors.New("正在整理中")
	}
	if err := s.db.WithContext(ctx).Model(&noteRow{}).Where("id = ?", row.ID).Updates(map[string]any{
		"extract_status": StatusQueued, "extract_attempts": 0, "claimed_at": nil, "extract_error": nil,
	}).Error; err != nil {
		return err
	}
	s.Wake()
	return nil
}

// Attachment 读出一张附件，前端查看原图用。
func (s *Service) Attachment(ctx context.Context, familyKey, noteKey string, index int) (Image, error) {
	row, err := s.getRow(ctx, familyKey, noteKey)
	if err != nil {
		return Image{}, err
	}
	attachments := row.attachments()
	if index < 0 || index >= len(attachments) {
		return Image{}, ErrNotFound
	}
	data, err := s.storage.Load(attachments[index].Path)
	if err != nil {
		return Image{}, err
	}
	return Image{MediaType: attachments[index].MediaType, Data: data}, nil
}

func (s *Service) getRow(ctx context.Context, familyKey, noteKey string) (noteRow, error) {
	var row noteRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND note_key = ?", familyKey, noteKey).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, ErrNotFound
		}
		return row, err
	}
	return row, nil
}

func (s *Service) loadImages(row noteRow) ([]llm.Block, error) {
	var blocks []llm.Block
	for _, attachment := range row.attachments() {
		data, err := s.storage.Load(attachment.Path)
		if err != nil {
			return nil, fmt.Errorf("读取附件 %s 失败: %w", attachment.Path, err)
		}
		blocks = append(blocks, llm.Block{Type: llm.BlockImage, MediaType: attachment.MediaType, Data: base64.StdEncoding.EncodeToString(data)})
	}
	return blocks, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
