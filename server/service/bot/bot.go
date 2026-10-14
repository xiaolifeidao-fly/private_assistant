// Package bot 管机器人：私人助理和三个分身（老婆、妈妈、女儿）。
//
// 机器人的系统提示词只写「它是谁、怎么做事」；家庭档案和记忆由 chat 在每轮对话时
// 拼进上下文，不写死在提示词里。
package bot

import (
	"context"
	"errors"
	"strings"
	"time"

	"common/middleware/db"

	"gorm.io/gorm"
)

const (
	KindAssistant = "assistant"
	KindPersona   = "persona"

	ScopeAll    = "all"
	ScopePerson = "person"
)

var ErrNotFound = errors.New("机器人不存在")

type Bot struct {
	BotKey       string    `json:"botKey"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	PersonKey    string    `json:"personKey"`
	Avatar       string    `json:"avatar"`
	Description  string    `json:"description"`
	Greeting     string    `json:"greeting"`
	SystemPrompt string    `json:"systemPrompt"`
	RouteTask    string    `json:"routeTask"`
	Tools        []string  `json:"tools"`
	MemoryScope  string    `json:"memoryScope"`
	Enabled      bool      `json:"enabled"`
	Sort         int       `json:"sort"`
	Version      int       `json:"version"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Service struct{ db *gorm.DB }

func New(database *gorm.DB) *Service { return &Service{db: database} }

// EnsureDefaults 在机器人不存在时写入默认配置；已存在的不动（管理端改过的提示词不会被覆盖）。
func (s *Service) EnsureDefaults(ctx context.Context, familyKey string) error {
	now := time.Now()
	for _, item := range defaults() {
		var count int64
		s.db.WithContext(ctx).Model(&botRow{}).Where("bot_key = ?", item.BotKey).Count(&count)
		if count > 0 {
			continue
		}
		row := toRow(familyKey, item)
		row.CreatedAt, row.UpdatedAt, row.Version = now, now, 1
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// DefaultPrompt 返回某个默认机器人的出厂提示词，管理端「恢复默认」用。
func DefaultPrompt(botKey string) (string, bool) {
	for _, item := range defaults() {
		if item.BotKey == botKey {
			return item.SystemPrompt, true
		}
	}
	return "", false
}

func (s *Service) List(ctx context.Context, familyKey string, onlyEnabled bool) ([]Bot, error) {
	query := s.db.WithContext(ctx).Where("family_key = ?", familyKey)
	if onlyEnabled {
		query = query.Where("enabled = 1")
	}
	var rows []botRow
	if err := query.Order("sort, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Bot, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, familyKey, botKey string) (Bot, error) {
	var row botRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND bot_key = ?", familyKey, botKey).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Bot{}, ErrNotFound
		}
		return Bot{}, err
	}
	return row.toDTO(), nil
}

// Update 修改机器人配置。提示词变化时版本号 +1，消息记录里能对上是哪一版提示词说的话。
func (s *Service) Update(ctx context.Context, familyKey, botKey string, input Bot) (Bot, error) {
	current, err := s.Get(ctx, familyKey, botKey)
	if err != nil {
		return Bot{}, err
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.SystemPrompt) == "" || input.RouteTask == "" {
		return Bot{}, errors.New("名称、提示词、模型路由不能为空")
	}
	if input.MemoryScope != ScopeAll && input.MemoryScope != ScopePerson {
		return Bot{}, errors.New("记忆范围只能是 all 或 person")
	}
	version := current.Version
	if input.SystemPrompt != current.SystemPrompt {
		version++
	}
	enabled := 0
	if input.Enabled {
		enabled = 1
	}
	updates := map[string]any{
		"name": strings.TrimSpace(input.Name), "avatar": input.Avatar, "description": input.Description,
		"greeting": input.Greeting, "system_prompt": input.SystemPrompt, "route_task": input.RouteTask,
		"tools": db.StringList(input.Tools), "memory_scope": input.MemoryScope, "enabled": enabled,
		"sort": input.Sort, "version": version, "updated_at": time.Now(),
	}
	if err := s.db.WithContext(ctx).Model(&botRow{}).Where("family_key = ? AND bot_key = ?", familyKey, botKey).
		Updates(updates).Error; err != nil {
		return Bot{}, err
	}
	return s.Get(ctx, familyKey, botKey)
}

type botRow struct {
	ID           int64
	BotKey       string
	FamilyKey    string
	Name         string
	Kind         string
	PersonKey    *string
	Avatar       string
	Description  string
	Greeting     string
	SystemPrompt string
	RouteTask    string
	Tools        db.StringList
	MemoryScope  string
	Enabled      int
	Sort         int
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (botRow) TableName() string { return "pa_bot" }

func (row botRow) toDTO() Bot {
	bot := Bot{
		BotKey: row.BotKey, Name: row.Name, Kind: row.Kind, Avatar: row.Avatar, Description: row.Description,
		Greeting: row.Greeting, SystemPrompt: row.SystemPrompt, RouteTask: row.RouteTask, Tools: []string(row.Tools),
		MemoryScope: row.MemoryScope, Enabled: row.Enabled == 1, Sort: row.Sort, Version: row.Version, UpdatedAt: row.UpdatedAt,
	}
	if row.PersonKey != nil {
		bot.PersonKey = *row.PersonKey
	}
	if bot.Tools == nil {
		bot.Tools = []string{}
	}
	return bot
}

func toRow(familyKey string, bot Bot) botRow {
	row := botRow{
		BotKey: bot.BotKey, FamilyKey: familyKey, Name: bot.Name, Kind: bot.Kind, Avatar: bot.Avatar,
		Description: bot.Description, Greeting: bot.Greeting, SystemPrompt: bot.SystemPrompt, RouteTask: bot.RouteTask,
		Tools: db.StringList(bot.Tools), MemoryScope: bot.MemoryScope, Enabled: 1, Sort: bot.Sort,
	}
	if bot.PersonKey != "" {
		personKey := bot.PersonKey
		row.PersonKey = &personKey
	}
	return row
}
