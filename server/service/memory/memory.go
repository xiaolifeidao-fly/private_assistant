// Package memory 是记忆系统：记忆的增删改查、状态流转（待确认 / 生效 / 被替代 / 归档）、
// 向量化和混合检索。
//
// 记忆可以演化，但不覆盖：事实变了用 Supersede —— 旧记忆标记 superseded 并写上失效时间，
// 新记忆指向旧记忆，演化轨迹完整保留。
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"common/keys"
	"common/middleware/db"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("记忆不存在")

// 状态
const (
	StatusPending    = "pending"
	StatusActive     = "active"
	StatusSuperseded = "superseded"
	StatusArchived   = "archived"
	StatusRejected   = "rejected"
)

// TypeInfo 描述一种记忆类型，前端下拉和提示词都用它。
type TypeInfo struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
	// Decays 为 true 的类型检索时按时间衰减（事件、原话会过时；性格、规矩不会）。
	Decays bool `json:"decays"`
}

var Types = []TypeInfo{
	{"trait", "性格特质", "相对稳定的性格特点，如：自尊心强，被当众否定会很难受", false},
	{"habit", "生活习惯", "作息、做事方式，如：每天 5 点半起床", false},
	{"preference", "喜好厌恶", "喜欢/不喜欢的东西，attrs.polarity=like/dislike", false},
	{"quote", "原话", "某人说过的原话，content 里尽量保留原文，attrs.speaker 为说话人", true},
	{"need", "诉求", "她在意、希望我做到的事，attrs.status=open/addressed", true},
	{"commitment", "我的承诺", "我答应过的事，attrs.promisedTo / attrs.dueAt / attrs.status=open/done", true},
	{"event", "事件", "发生过的事情经过、各方反应和结果", true},
	{"conflict", "矛盾点", "长期存在的分歧，attrs.parties / attrs.status=open/eased/resolved", false},
	{"taboo", "雷区", "绝对不要做/说的事", false},
	{"custom", "规矩习俗", "某个家/老家的规矩：拜年、红包、座次、送礼", false},
	{"routine", "作息安排", "孩子的作息、学习、固定安排", false},
	{"health", "健康", "过敏、疾病、用药", false},
	{"milestone", "成长节点", "孩子的成长节点、家里的大事", false},
	{"item", "物品", "家里的东西：是什么、干什么用、放在哪。attrs.location=存放位置，attrs.usage=用途，attrs.photos=原始记录里对应的照片序号；挪了地方用新记忆替代旧的", false},
	{"insight", "归纳洞察", "系统周期性归纳出的结论", true},
}

func typeInfo(memoryType string) (TypeInfo, bool) {
	for _, info := range Types {
		if info.Type == memoryType {
			return info, true
		}
	}
	return TypeInfo{}, false
}

// TypeLabel 返回类型的中文名，未知类型原样返回。
func TypeLabel(memoryType string) string {
	if info, ok := typeInfo(memoryType); ok {
		return info.Label
	}
	return memoryType
}

// Embedder 由 llm 网关实现；未配置 embedding 时检索退化为全文 + 结构化。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, string, error)
	// EmbedQuery 给检索的问题算向量（有的模型存入和查询的编码方式不同）
	EmbedQuery(ctx context.Context, text string) ([]float32, string, error)
	EmbeddingModel(ctx context.Context) string
}

// PersonLabels 返回人物 key → 称呼（「老婆」「妈妈」……），算向量时把「关于谁」写成文字。由 wire 注入。
type PersonLabels func(ctx context.Context, familyKey string) map[string]string

type Service struct {
	db       *gorm.DB
	embedder Embedder
	vectors  VectorStore
	labels   PersonLabels
	index    *vectorIndex
	passages *vectorIndex // 原文片段的向量（passage.go）

	queryMu    sync.Mutex
	queryCache map[string]cachedQuery
}

// New 的 vectors 为 nil 时向量存 MySQL（pa_memory_embedding）；labels 可为 nil。
func New(database *gorm.DB, embedder Embedder, vectors VectorStore, labels PersonLabels) *Service {
	if vectors == nil {
		vectors = MySQLVectorStore{DB: database}
	}
	return &Service{db: database, embedder: embedder, vectors: vectors, labels: labels, index: newVectorIndex(), passages: newVectorIndex(),
		queryCache: map[string]cachedQuery{}}
}

type Memory struct {
	MemoryKey     string          `json:"memoryKey"`
	Type          string          `json:"type"`
	TypeLabel     string          `json:"typeLabel"`
	Scope         string          `json:"scope"`
	SubjectPerson string          `json:"subjectPerson"`
	PlaceKey      string          `json:"placeKey"`
	Persons       []string        `json:"persons"`
	Title         string          `json:"title"`
	Content       string          `json:"content"`
	Attrs         json.RawMessage `json:"attrs"`
	Importance    int             `json:"importance"`
	Confidence    float64         `json:"confidence"`
	Status        string          `json:"status"`
	OccurredAt    *time.Time      `json:"occurredAt"`
	ValidFrom     *time.Time      `json:"validFrom"`
	ValidTo       *time.Time      `json:"validTo"`
	SupersedesKey string          `json:"supersedesKey"`
	SourceNoteKey string          `json:"sourceNoteKey"`
	CreatedBy     string          `json:"createdBy"`
	UseCount      int             `json:"useCount"`
	LastUsedAt    *time.Time      `json:"lastUsedAt"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type Input struct {
	Type          string          `json:"type"`
	Scope         string          `json:"scope"`
	SubjectPerson string          `json:"subjectPerson"`
	PlaceKey      string          `json:"placeKey"`
	Persons       []string        `json:"persons"`
	Title         string          `json:"title"`
	Content       string          `json:"content"`
	Attrs         json.RawMessage `json:"attrs"`
	Importance    int             `json:"importance"`
	Confidence    float64         `json:"confidence"`
	Status        string          `json:"status"`
	OccurredAt    *time.Time      `json:"occurredAt"`
	ValidFrom     *time.Time      `json:"validFrom"`
	CreatedBy     string          `json:"createdBy"`
	SourceNoteKey string          `json:"sourceNoteKey"`
	// SupersedesKey 非空表示「这条要替代那条旧记忆」。只在确认（pending→active）时才真正
	// 把旧记忆标成 superseded —— 「她变了」这种判断必须由我拍板。
	SupersedesKey string `json:"supersedesKey"`
}

func (s *Service) Create(ctx context.Context, familyKey string, input Input) (Memory, error) {
	row, err := s.buildRow(familyKey, input)
	if err != nil {
		return Memory{}, err
	}
	now := time.Now()
	row.MemoryKey = keys.New("mem")
	row.CreatedAt, row.UpdatedAt = now, now
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return replaceLinks(tx, row.MemoryKey, linkTargets(input))
	})
	if err != nil {
		return Memory{}, err
	}
	s.embedAsync(familyKey, row)
	return s.Get(ctx, familyKey, row.MemoryKey)
}

func (s *Service) Update(ctx context.Context, familyKey, memoryKey string, input Input) (Memory, error) {
	existing, err := s.getRow(ctx, familyKey, memoryKey)
	if err != nil {
		return Memory{}, err
	}
	if input.Status == "" {
		input.Status = existing.Status
	}
	if input.CreatedBy == "" {
		input.CreatedBy = existing.CreatedBy
	}
	row, err := s.buildRow(familyKey, input)
	if err != nil {
		return Memory{}, err
	}
	row.UpdatedAt = time.Now()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&memoryRow{}).Where("id = ?", existing.ID).
			Select("type", "scope", "subject_person", "place_key", "title", "content", "attrs", "importance",
				"confidence", "status", "occurred_at", "valid_from", "updated_at").
			Updates(&row).Error; err != nil {
			return err
		}
		return replaceLinks(tx, memoryKey, linkTargets(input))
	})
	if err != nil {
		return Memory{}, err
	}
	// 算向量的文字里有类型、主体、位置等（embeddingText），这些变了也要重算
	if row.Title != existing.Title || row.Content != existing.Content || row.Type != existing.Type ||
		deref(row.SubjectPerson) != deref(existing.SubjectPerson) || string(row.Attrs) != string(existing.Attrs) {
		row.MemoryKey = memoryKey
		s.embedAsync(familyKey, row)
	}
	return s.Get(ctx, familyKey, memoryKey)
}

// SetStatus 用于确认（pending→active）、驳回、归档、恢复。
// 确认一条带 SupersedesKey 的记忆时，同时把它要替代的旧记忆标记为 superseded。
func (s *Service) SetStatus(ctx context.Context, familyKey, memoryKey, status string) error {
	switch status {
	case StatusActive, StatusArchived, StatusRejected, StatusPending:
	default:
		return fmt.Errorf("不支持的状态 %s", status)
	}
	row, err := s.getRow(ctx, familyKey, memoryKey)
	if err != nil {
		return err
	}
	now := time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": status, "updated_at": now}
		if status == StatusActive && row.SupersedesKey != nil {
			result := tx.Model(&memoryRow{}).
				Where("family_key = ? AND memory_key = ? AND status = ?", familyKey, *row.SupersedesKey, StatusActive).
				Updates(map[string]any{"status": StatusSuperseded, "valid_to": now, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if row.ValidFrom == nil {
				updates["valid_from"] = now
			}
		}
		return tx.Model(&memoryRow{}).Where("id = ?", row.ID).Updates(updates).Error
	})
}

// Supersede 用新记忆替代旧记忆：旧的标记 superseded、valid_to=now，新的指向旧的。
// 用于「她变了」——性格、习惯、喜好发生变化时不覆盖历史。
func (s *Service) Supersede(ctx context.Context, familyKey, oldKey string, input Input) (Memory, error) {
	old, err := s.getRow(ctx, familyKey, oldKey)
	if err != nil {
		return Memory{}, err
	}
	if old.Status == StatusSuperseded {
		return Memory{}, errors.New("这条记忆已经被替代过了")
	}
	row, err := s.buildRow(familyKey, input)
	if err != nil {
		return Memory{}, err
	}
	now := time.Now()
	row.MemoryKey = keys.New("mem")
	row.SupersedesKey = &old.MemoryKey
	if row.ValidFrom == nil {
		row.ValidFrom = &now
	}
	row.CreatedAt, row.UpdatedAt = now, now
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&memoryRow{}).Where("id = ?", old.ID).
			Updates(map[string]any{"status": StatusSuperseded, "valid_to": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return replaceLinks(tx, row.MemoryKey, linkTargets(input))
	})
	if err != nil {
		return Memory{}, err
	}
	s.embedAsync(familyKey, row)
	return s.Get(ctx, familyKey, row.MemoryKey)
}

// History 返回一条记忆的完整替代链（从最早到最新）。
func (s *Service) History(ctx context.Context, familyKey, memoryKey string) ([]Memory, error) {
	current, err := s.getRow(ctx, familyKey, memoryKey)
	if err != nil {
		return nil, err
	}
	chain := []memoryRow{current}
	// 往前追
	for cursor := current; cursor.SupersedesKey != nil && len(chain) < 50; {
		previous, err := s.getRow(ctx, familyKey, *cursor.SupersedesKey)
		if err != nil {
			break
		}
		chain = append([]memoryRow{previous}, chain...)
		cursor = previous
	}
	// 往后追
	for cursor := current; len(chain) < 50; {
		var next memoryRow
		if err := s.db.WithContext(ctx).Where("family_key = ? AND supersedes_key = ?", familyKey, cursor.MemoryKey).First(&next).Error; err != nil {
			break
		}
		chain = append(chain, next)
		cursor = next
	}
	return s.toDTOs(ctx, chain)
}

func (s *Service) Get(ctx context.Context, familyKey, memoryKey string) (Memory, error) {
	row, err := s.getRow(ctx, familyKey, memoryKey)
	if err != nil {
		return Memory{}, err
	}
	result, err := s.toDTOs(ctx, []memoryRow{row})
	if err != nil {
		return Memory{}, err
	}
	return result[0], nil
}

type ListFilter struct {
	Types  []string
	Person string // 关联到某人（主体或关联人）
	// VisibleTo 非空时只返回该人的分身可见记忆：关于本人的，以及公共/地点记忆。
	// 它不是 API 查询条件，只由服务端在分身工具中设置。
	VisibleTo string
	Statuses  []string
	Keyword   string
	Source    string // 来自哪条原始记录
	Page      int
	Size      int
}

func (s *Service) List(ctx context.Context, familyKey string, filter ListFilter) ([]Memory, int64, error) {
	query := s.db.WithContext(ctx).Model(&memoryRow{}).Where("family_key = ?", familyKey)
	if len(filter.Types) > 0 {
		query = query.Where("type IN ?", filter.Types)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	if filter.Person != "" {
		query = query.Where("(subject_person = ? OR memory_key IN (SELECT memory_key FROM pa_memory_link WHERE target_type = 'person' AND target_key = ?))", filter.Person, filter.Person)
	}
	if filter.VisibleTo != "" {
		query = query.Where("(subject_person = ? OR subject_person IS NULL OR scope IN ('shared','place'))", filter.VisibleTo)
	}
	if filter.Source != "" {
		query = query.Where("source_note_key = ?", filter.Source)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("(title LIKE ? OR content LIKE ?)", like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	size := filter.Size
	if size <= 0 || size > 200 {
		size = 30
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	var rows []memoryRow
	if err := query.Order("COALESCE(occurred_at, created_at) DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	result, err := s.toDTOs(ctx, rows)
	return result, total, err
}

// LatestChange 返回关于某人的生效记忆条数和最近一次变化时间（人设卡据此判断要不要刷新）。
func (s *Service) LatestChange(ctx context.Context, familyKey, personKey string) (int64, *time.Time, error) {
	var result struct {
		Count  int64
		Latest *time.Time
	}
	err := s.db.WithContext(ctx).Model(&memoryRow{}).
		Select("COUNT(*) AS count, MAX(updated_at) AS latest").
		Where("family_key = ? AND status IN ?", familyKey, []string{StatusActive, StatusSuperseded}).
		Where("(subject_person = ? OR memory_key IN (SELECT memory_key FROM pa_memory_link WHERE target_type = 'person' AND target_key = ?))", personKey, personKey).
		Scan(&result).Error
	return result.Count, result.Latest, err
}

// CountPending 返回待确认的记忆数，App 角标用。
func (s *Service) CountPending(ctx context.Context, familyKey string) int64 {
	var count int64
	s.db.WithContext(ctx).Model(&memoryRow{}).Where("family_key = ? AND status = ?", familyKey, StatusPending).Count(&count)
	return count
}

// MarkUsed 记录记忆被检索命中，用于排序和日后清理冷记忆。
func (s *Service) MarkUsed(ctx context.Context, memoryKeys []string) {
	if len(memoryKeys) == 0 {
		return
	}
	s.db.WithContext(ctx).Model(&memoryRow{}).Where("memory_key IN ?", memoryKeys).
		Updates(map[string]any{"use_count": gorm.Expr("use_count + 1"), "last_used_at": time.Now()})
}

// ---------------------------------------------------------------- internal

func (s *Service) getRow(ctx context.Context, familyKey, memoryKey string) (memoryRow, error) {
	var row memoryRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND memory_key = ?", familyKey, memoryKey).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, ErrNotFound
		}
		return row, err
	}
	return row, nil
}

func (s *Service) buildRow(familyKey string, input Input) (memoryRow, error) {
	if _, ok := typeInfo(input.Type); !ok {
		return memoryRow{}, fmt.Errorf("未知的记忆类型 %s", input.Type)
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	if input.Title == "" && input.Content == "" {
		return memoryRow{}, errors.New("记忆内容不能为空")
	}
	if input.Title == "" {
		input.Title = summarize(input.Content, 60)
	}
	if input.Content == "" {
		input.Content = input.Title
	}
	scope := input.Scope
	if scope == "" {
		scope = "person"
		if input.SubjectPerson == "" {
			scope = "shared"
		}
		if input.Type == "custom" && input.PlaceKey != "" {
			scope = "place"
		}
	}
	status := input.Status
	if status == "" {
		status = StatusActive
	}
	confidence := input.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = 1
	}
	importance := input.Importance
	if importance < 1 || importance > 5 {
		importance = 3
	}
	if len(input.Attrs) > 0 && !json.Valid(input.Attrs) {
		return memoryRow{}, errors.New("attrs 不是合法的 JSON")
	}
	return memoryRow{
		FamilyKey: familyKey, Type: input.Type, Scope: scope, SubjectPerson: nullable(input.SubjectPerson),
		PlaceKey: nullable(input.PlaceKey), Title: summarize(input.Title, 200), Content: input.Content,
		Attrs: db.JSON(input.Attrs), Importance: importance, Confidence: confidence, Status: status,
		OccurredAt: input.OccurredAt, ValidFrom: input.ValidFrom, SourceNoteKey: nullable(input.SourceNoteKey),
		SupersedesKey: nullable(input.SupersedesKey),
		CreatedBy:     defaultString(input.CreatedBy, "manual"),
	}, nil
}

func linkTargets(input Input) []linkRow {
	var links []linkRow
	seen := map[string]bool{}
	add := func(targetType, key string) {
		key = strings.TrimSpace(key)
		if key == "" || seen[targetType+key] {
			return
		}
		seen[targetType+key] = true
		links = append(links, linkRow{TargetType: targetType, TargetKey: key})
	}
	add("person", input.SubjectPerson)
	for _, person := range input.Persons {
		add("person", person)
	}
	add("place", input.PlaceKey)
	return links
}

func replaceLinks(tx *gorm.DB, memoryKey string, links []linkRow) error {
	if err := tx.Where("memory_key = ?", memoryKey).Delete(&linkRow{}).Error; err != nil {
		return err
	}
	for index := range links {
		links[index].MemoryKey = memoryKey
	}
	if len(links) == 0 {
		return nil
	}
	return tx.Create(&links).Error
}

func (s *Service) toDTOs(ctx context.Context, rows []memoryRow) ([]Memory, error) {
	result := make([]Memory, 0, len(rows))
	if len(rows) == 0 {
		return result, nil
	}
	memoryKeys := make([]string, 0, len(rows))
	for _, row := range rows {
		memoryKeys = append(memoryKeys, row.MemoryKey)
	}
	var links []linkRow
	if err := s.db.WithContext(ctx).Where("memory_key IN ? AND target_type = 'person'", memoryKeys).Order("id").Find(&links).Error; err != nil {
		return nil, err
	}
	persons := map[string][]string{}
	for _, link := range links {
		persons[link.MemoryKey] = append(persons[link.MemoryKey], link.TargetKey)
	}
	for _, row := range rows {
		item := row.toDTO()
		item.Persons = persons[row.MemoryKey]
		if item.Persons == nil {
			item.Persons = []string{}
		}
		result = append(result, item)
	}
	return result, nil
}

func summarize(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return value
}

func nullable(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
