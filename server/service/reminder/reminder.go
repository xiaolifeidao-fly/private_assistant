package reminder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"common/keys"
	"common/middleware/db"
	"common/viewer"
	"service/memory"

	"gorm.io/gorm"
)

const (
	StatusScheduled = "scheduled"
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusFailed    = "failed"
	StatusDismissed = "dismissed"
	StatusCancelled = "cancelled"
)

var ErrNotFound = errors.New("提醒不存在")

type Config struct {
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
}

type Service struct {
	db     *gorm.DB
	family FamilyPort
	memory MemoryPort
	config Config
}

func New(database *gorm.DB, familyService FamilyPort, memoryService MemoryPort, config Config) *Service {
	return &Service{db: database, family: familyService, memory: memoryService, config: config}
}

type Reminder struct {
	ReminderKey string     `json:"reminderKey"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	EventAt     *time.Time `json:"eventAt"`
	RemindAt    time.Time  `json:"remindAt"`
	Status      string     `json:"status"`
	SourceType  string     `json:"sourceType"`
	SourceKey   string     `json:"sourceKey"`
	// OwnerAccount / Visibility：谁的提醒、谁能看到；推送只发给能看到的账号（common/viewer）
	OwnerAccount string          `json:"ownerAccount"`
	Visibility   string          `json:"visibility"`
	TargetURL    string          `json:"targetUrl"`
	Payload      json.RawMessage `json:"payload"`
	Attempts     int             `json:"attempts"`
	LastError    string          `json:"lastError"`
	SentAt       *time.Time      `json:"sentAt"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

type Trip struct {
	TripKey   string     `json:"tripKey"`
	PlaceKey  string     `json:"placeKey"`
	Title     string     `json:"title"`
	StartAt   time.Time  `json:"startAt"`
	EndAt     *time.Time `json:"endAt"`
	People    []string   `json:"people"`
	Notes     string     `json:"notes"`
	Status    string     `json:"status"`
	Brief     string     `json:"brief,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

type ManualInput struct {
	// Kind 默认 manual；别的模块自动建的提醒可以标上自己的类型（weather 宝宝穿衣）
	Kind      string     `json:"-"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	EventAt   *time.Time `json:"eventAt"`
	RemindAt  time.Time  `json:"remindAt"`
	TargetURL string     `json:"targetUrl"`
	// Visibility private（默认，只提醒自己）/ family（全家都提醒）
	Visibility string `json:"visibility"`
}

func (s *Service) PublicKey() string { return s.config.VAPIDPublicKey }
func (s *Service) PushEnabled() bool {
	return s.config.VAPIDPublicKey != "" && s.config.VAPIDPrivateKey != "" && s.config.VAPIDSubject != ""
}

func (s *Service) List(ctx context.Context, familyKey string, from, to time.Time, includeHistory bool) ([]Reminder, error) {
	query := s.db.WithContext(ctx).Where("family_key = ?", familyKey)
	if !from.IsZero() {
		query = query.Where("remind_at >= ?", from)
	}
	if !to.IsZero() {
		query = query.Where("remind_at < ?", to)
	}
	if !includeHistory {
		query = query.Where("status IN ?", []string{StatusScheduled, StatusFailed, StatusSent})
	}
	var rows []reminderRow
	if err := query.Order("remind_at, id").Limit(500).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Reminder, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

func (s *Service) Upcoming(ctx context.Context, familyKey string, days int) ([]Reminder, error) {
	if days <= 0 || days > 366 {
		days = 30
	}
	now := time.Now().In(shanghai)
	return s.List(ctx, familyKey, now.Add(-24*time.Hour), now.AddDate(0, 0, days+1), false)
}

func (s *Service) CreateManual(ctx context.Context, familyKey string, input ManualInput) (Reminder, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || input.RemindAt.IsZero() {
		return Reminder{}, errors.New("提醒标题和时间不能为空")
	}
	kind := input.Kind
	if kind == "" {
		kind = "manual"
	}
	return s.upsert(ctx, familyKey, reminderSeed{Kind: kind, Title: input.Title, Body: input.Body,
		EventAt: input.EventAt, RemindAt: input.RemindAt.In(shanghai), DedupeKey: keys.New("manual"), TargetURL: input.TargetURL,
		Visibility: viewer.Resolve(ctx, input.Visibility)})
}

func (s *Service) Dismiss(ctx context.Context, familyKey, reminderKey string) error {
	result := s.db.WithContext(ctx).Model(&reminderRow{}).Where("family_key = ? AND reminder_key = ?", familyKey, reminderKey).
		Updates(map[string]any{"status": StatusDismissed, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Owner / Visibility 为空时：归当前账号（后台里是系统账号），私有。
type reminderSeed struct {
	Kind, Title, Body, SourceType, SourceKey, DedupeKey, TargetURL string
	Owner, Visibility                                              string
	EventAt                                                        *time.Time
	RemindAt                                                       time.Time
	Payload                                                        any
}

func (s *Service) upsert(ctx context.Context, familyKey string, seed reminderSeed) (Reminder, error) {
	var existing reminderRow
	err := s.db.WithContext(ctx).Where("family_key = ? AND dedupe_key = ?", familyKey, seed.DedupeKey).First(&existing).Error
	payload := db.MustJSON(seed.Payload)
	now := time.Now()
	if seed.Owner == "" {
		seed.Owner = viewer.Owner(ctx)
	}
	if seed.Visibility != viewer.Family {
		seed.Visibility = viewer.Private
	}
	if err == nil {
		if existing.Status == StatusScheduled || existing.Status == StatusFailed {
			updates := map[string]any{"title": seed.Title, "body": seed.Body, "event_at": seed.EventAt, "remind_at": seed.RemindAt,
				"target_url": seed.TargetURL, "payload": payload, "visibility": seed.Visibility, "updated_at": now}
			if err := s.db.WithContext(ctx).Model(&existing).Updates(updates).Error; err != nil {
				return Reminder{}, err
			}
			return s.get(ctx, familyKey, existing.ReminderKey)
		}
		return existing.toDTO(), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Reminder{}, err
	}
	row := reminderRow{ReminderKey: keys.New("rmd"), FamilyKey: familyKey, Kind: seed.Kind, Title: seed.Title, Body: seed.Body,
		EventAt: seed.EventAt, RemindAt: seed.RemindAt, Status: StatusScheduled, SourceType: seed.SourceType, SourceKey: seed.SourceKey,
		OwnerAccount: seed.Owner, Visibility: seed.Visibility,
		DedupeKey: seed.DedupeKey, TargetURL: seed.TargetURL, Payload: payload, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		// 两个 worker 同时派生同一条时，唯一键冲突后读已有结果。
		var raced reminderRow
		if findErr := s.db.WithContext(ctx).Where("family_key = ? AND dedupe_key = ?", familyKey, seed.DedupeKey).First(&raced).Error; findErr == nil {
			return raced.toDTO(), nil
		}
		return Reminder{}, err
	}
	return row.toDTO(), nil
}

func (s *Service) get(ctx context.Context, familyKey, reminderKey string) (Reminder, error) {
	var row reminderRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND reminder_key = ?", familyKey, reminderKey).First(&row).Error; err != nil {
		return Reminder{}, ErrNotFound
	}
	return row.toDTO(), nil
}

// SetVisibility 改提醒的可见范围：private 只提醒自己，family 全家都提醒。只有归属人自己能改。
func (s *Service) SetVisibility(ctx context.Context, familyKey, reminderKey, visibility string) (Reminder, error) {
	if visibility != viewer.Private && visibility != viewer.Family {
		return Reminder{}, errors.New("可见范围只能是 private 或 family")
	}
	item, err := s.get(ctx, familyKey, reminderKey)
	if err != nil {
		return Reminder{}, err
	}
	if !viewer.CanChange(ctx, item.OwnerAccount) {
		return Reminder{}, errors.New("只有设这条提醒的人能改谁能收到")
	}
	if err := s.db.WithContext(ctx).Model(&reminderRow{}).Where("family_key = ? AND reminder_key = ?", familyKey, reminderKey).
		Updates(map[string]any{"visibility": visibility, "updated_at": time.Now()}).Error; err != nil {
		return Reminder{}, err
	}
	return s.get(ctx, familyKey, reminderKey)
}

func endpointHash(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

func attrDue(item memory.Memory) (time.Time, bool) {
	var attrs struct {
		DueAt  string `json:"dueAt"`
		Status string `json:"status"`
	}
	if json.Unmarshal(item.Attrs, &attrs) != nil || attrs.DueAt == "" || attrs.Status == "done" || attrs.Status == "addressed" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, attrs.DueAt, shanghai); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
