// Package skill 管做事手册：遇到某类事该怎么想、先查什么、怎么说、别做什么。
//
// 系统提示词里只放目录（Index），正文由模型调用 load_skill 时读取，作为工具结果
// 追加进对话历史 —— 不改动冻结的系统提示词，读过一次整个会话都在。
package skill

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"common/middleware/db"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("没有这本手册")

type Skill struct {
	SkillKey  string    `json:"skillKey"`
	Name      string    `json:"name"`
	WhenToUse string    `json:"whenToUse"`
	Body      string    `json:"body"`
	BotKeys   []string  `json:"botKeys"`
	Enabled   bool      `json:"enabled"`
	Sort      int       `json:"sort"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AvailableTo 判断手册是否对某个机器人开放。
func (s Skill) AvailableTo(botKey string) bool {
	return s.Enabled && slices.Contains(s.BotKeys, botKey)
}

type Service struct {
	db     *gorm.DB
	family FamilyPort
	memory MemoryPort
	llm    LLMPort
}

func New(database *gorm.DB, familyService FamilyPort, memoryService MemoryPort, gateway LLMPort) *Service {
	return &Service{db: database, family: familyService, memory: memoryService, llm: gateway}
}

// EnsureDefaults 写入还不存在的出厂手册；已存在的不动（管理端改过的不会被覆盖）。
func (s *Service) EnsureDefaults(ctx context.Context, familyKey string) error {
	now := time.Now()
	for _, item := range defaults() {
		var count int64
		s.db.WithContext(ctx).Model(&skillRow{}).Where("skill_key = ?", item.SkillKey).Count(&count)
		if count > 0 {
			continue
		}
		row := toRow(familyKey, item)
		row.Enabled, row.Version, row.CreatedAt, row.UpdatedAt = 1, 1, now, now
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// Default 返回出厂内容，管理端「恢复默认」用。
func Default(skillKey string) (Skill, bool) {
	for _, item := range defaults() {
		if item.SkillKey == skillKey {
			return item, true
		}
	}
	return Skill{}, false
}

func (s *Service) List(ctx context.Context, familyKey string) ([]Skill, error) {
	var rows []skillRow
	if err := s.db.WithContext(ctx).Where("family_key = ?", familyKey).Order("sort, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Skill, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

// ForBot 返回对某个机器人开放的手册，按排序。
func (s *Service) ForBot(ctx context.Context, familyKey, botKey string) ([]Skill, error) {
	all, err := s.List(ctx, familyKey)
	if err != nil {
		return nil, err
	}
	var result []Skill
	for _, item := range all {
		if item.AvailableTo(botKey) {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, familyKey, skillKey string) (Skill, error) {
	var row skillRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND skill_key = ?", familyKey, skillKey).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Skill{}, ErrNotFound
		}
		return Skill{}, err
	}
	return row.toDTO(), nil
}

// Save 新建或修改手册。正文或适用条件变化时版本 +1，对话里能对上是哪一版。
func (s *Service) Save(ctx context.Context, familyKey string, input Skill) (Skill, error) {
	input.SkillKey = strings.TrimSpace(input.SkillKey)
	input.Name = strings.TrimSpace(input.Name)
	input.WhenToUse = strings.TrimSpace(input.WhenToUse)
	if !validKey(input.SkillKey) {
		return Skill{}, errors.New("标识只能用小写字母、数字和下划线，3-40 位")
	}
	if input.Name == "" || input.WhenToUse == "" || strings.TrimSpace(input.Body) == "" {
		return Skill{}, errors.New("名称、何时用、正文都不能为空")
	}
	enabled := 0
	if input.Enabled {
		enabled = 1
	}
	now := time.Now()
	current, err := s.Get(ctx, familyKey, input.SkillKey)
	if errors.Is(err, ErrNotFound) {
		row := toRow(familyKey, input)
		row.Enabled, row.Version, row.CreatedAt, row.UpdatedAt = enabled, 1, now, now
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return Skill{}, err
		}
		return s.Get(ctx, familyKey, input.SkillKey)
	}
	if err != nil {
		return Skill{}, err
	}
	version := current.Version
	if input.Body != current.Body || input.WhenToUse != current.WhenToUse || !slices.Equal(input.BotKeys, current.BotKeys) {
		version++
	}
	if err := s.db.WithContext(ctx).Model(&skillRow{}).Where("family_key = ? AND skill_key = ?", familyKey, input.SkillKey).
		Updates(map[string]any{
			"name": input.Name, "when_to_use": input.WhenToUse, "body": input.Body, "bot_keys": db.StringList(input.BotKeys),
			"enabled": enabled, "sort": input.Sort, "version": version, "updated_at": now,
		}).Error; err != nil {
		return Skill{}, err
	}
	return s.Get(ctx, familyKey, input.SkillKey)
}

func validKey(key string) bool {
	if len(key) < 3 || len(key) > 40 {
		return false
	}
	for _, r := range key {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

type skillRow struct {
	ID        int64
	SkillKey  string
	FamilyKey string
	Name      string
	WhenToUse string
	Body      string
	BotKeys   db.StringList
	Enabled   int
	Sort      int
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (skillRow) TableName() string { return "pa_skill" }

func (row skillRow) toDTO() Skill {
	item := Skill{
		SkillKey: row.SkillKey, Name: row.Name, WhenToUse: row.WhenToUse, Body: row.Body, BotKeys: []string(row.BotKeys),
		Enabled: row.Enabled == 1, Sort: row.Sort, Version: row.Version, UpdatedAt: row.UpdatedAt,
	}
	if item.BotKeys == nil {
		item.BotKeys = []string{}
	}
	return item
}

func toRow(familyKey string, item Skill) skillRow {
	return skillRow{
		SkillKey: item.SkillKey, FamilyKey: familyKey, Name: item.Name, WhenToUse: item.WhenToUse, Body: item.Body,
		BotKeys: db.StringList(item.BotKeys), Sort: item.Sort,
	}
}
