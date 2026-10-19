package diet

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"common/keys"
	"common/viewer"
	"service/llm"
	"service/reminder"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 每周计划对照实际：周日晚上把这一周每天的计划和实际、下周已经排的计划交给模型，写一段对照点评，
// 存进 pa_diet_review，推给能改计划的账号（我和老婆）。奶奶不推——她是照着做的人，点评是给排计划的人看的。
// 页面上也能手动生成（覆盖这一周已有的）。
const (
	taskReview       = "diet.review"
	reviewHour       = 20 // 周日晚上 8 点以后
	reviewTick       = 15 * time.Minute
	reviewMinRecords = 3 // 这周记得太少就不点评了，免得空对空
)

var errAlreadyReviewed = errors.New("这周已经点评过了")

type reviewRow struct {
	ID        int64
	ReviewKey string
	FamilyKey string
	ChildKey  string
	WeekStart time.Time
	Content   string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (reviewRow) TableName() string { return "pa_diet_review" }

type Review struct {
	ReviewKey string    `json:"reviewKey"`
	ChildKey  string    `json:"childKey"`
	WeekStart string    `json:"weekStart"`
	Content   string    `json:"content"`
	CreatedBy string    `json:"createdBy"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (row reviewRow) toDTO() Review {
	return Review{ReviewKey: row.ReviewKey, ChildKey: row.ChildKey, WeekStart: row.WeekStart.Format(dateLayout),
		Content: row.Content, CreatedBy: row.CreatedBy, UpdatedAt: row.UpdatedAt}
}

// ReminderPort 用来把点评推到手机（service/reminder）。
type ReminderPort interface {
	CreateManual(ctx context.Context, familyKey string, input reminder.ManualInput) (reminder.Reminder, error)
}

// SetNotifier 接上推送：reminders 建提醒，accounts 返回该推给谁（能改计划的账号）。由 wire 注入，没接就只存不推。
func (s *Service) SetNotifier(reminders ReminderPort, accounts func(ctx context.Context) []string) {
	s.reminders, s.planAccounts = reminders, accounts
}

// weekStartOf 返回 date 所在那周的周一。
func weekStartOf(date time.Time) time.Time {
	return date.AddDate(0, 0, -((int(date.Weekday()) + 6) % 7))
}

// GetReview 返回 date 所在那周的点评，没有返回 nil。
func (s *Service) GetReview(ctx context.Context, familyKey, childKey, date string) (*Review, error) {
	childKey, _, err := s.child(ctx, familyKey, childKey)
	if err != nil {
		return nil, err
	}
	day, err := parseDate(date)
	if err != nil {
		return nil, err
	}
	var row reviewRow
	err = s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND week_start = ?", familyKey, childKey,
		weekStartOf(day).Format(dateLayout)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && row.Content == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	review := row.toDTO()
	return &review, nil
}

// CreateReview 手动生成 date 所在那周的点评（覆盖已有的）。只有能改计划的账号能用。
func (s *Service) CreateReview(ctx context.Context, familyKey, childKey, date string, actor Actor) (Review, error) {
	if !actor.CanPlan() {
		return Review{}, ErrForbidden
	}
	day, err := parseDate(date)
	if err != nil {
		return Review{}, err
	}
	return s.review(ctx, familyKey, childKey, weekStartOf(day), actor.Account, true)
}

// review 生成一周的点评。force=false 时（定时任务）这周有了就不再生成。
func (s *Service) review(ctx context.Context, familyKey, childKey string, weekStart time.Time, by string, force bool) (result Review, err error) {
	person, err := s.childPerson(ctx, familyKey, childKey)
	if err != nil {
		return Review{}, err
	}
	now := time.Now()
	row := reviewRow{ReviewKey: keys.New("drev"), FamilyKey: familyKey, ChildKey: person.PersonKey, WeekStart: weekStart,
		CreatedBy: by, CreatedAt: now, UpdatedAt: now}
	// 先占位：两个进程都跑定时任务时只有一个能插进去
	claim := s.db.WithContext(ctx).Clauses(clause.Insert{Modifier: "IGNORE"}).Create(&row)
	if claim.Error != nil {
		return Review{}, claim.Error
	}
	if claim.RowsAffected > 0 {
		// 没写成（模型出错、这周记得太少）就把占位删掉，下次定时还会再试；手动生成失败也不留空行
		defer func() {
			if err != nil {
				s.db.WithContext(context.WithoutCancel(ctx)).Where("id = ? AND (content IS NULL OR content = '')", row.ID).Delete(&reviewRow{})
			}
		}()
	} else {
		if !force {
			return Review{}, errAlreadyReviewed
		}
		if err := s.db.WithContext(ctx).Where("family_key = ? AND child_key = ? AND week_start = ?", familyKey, person.PersonKey,
			weekStart.Format(dateLayout)).First(&row).Error; err != nil {
			return Review{}, err
		}
	}

	week, err := s.Overview(ctx, familyKey, person.PersonKey, weekStart.Format(dateLayout), 7, Actor{})
	if err != nil {
		return Review{}, err
	}
	if len(week.Records) < reviewMinRecords && !force {
		return Review{}, fmt.Errorf("这周只记了 %d 条，不点评", len(week.Records))
	}
	next, err := s.Overview(ctx, familyKey, person.PersonKey, weekStart.AddDate(0, 0, 7).Format(dateLayout), 7, Actor{})
	if err != nil {
		return Review{}, err
	}
	today, _ := parseDate("")
	var input strings.Builder
	fmt.Fprintf(&input, "孩子：%s", person.RoleLabel)
	if age := ageOf(person.Birthday, today); age != "" {
		fmt.Fprintf(&input, "，%s", age)
	}
	fmt.Fprintf(&input, "\n今天：%s\n\n【这一周】\n%s\n【下一周已经排的计划】\n", today.Format(dateLayout), formatOverview(week))
	if len(next.Plans) == 0 {
		input.WriteString("（还没排）\n")
	} else {
		input.WriteString(formatOverview(next))
	}
	response, err := s.llm.Chat(ctx, llm.Request{
		Task:     taskReview,
		System:   []llm.SystemPart{{Text: reviewSystem, Cache: true}},
		Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: input.String()}}}},
		RefKey:   row.ReviewKey,
	}, nil)
	if err != nil {
		return Review{}, err
	}
	content := strings.TrimSpace(response.Message.Text())
	if content == "" {
		return Review{}, errors.New("模型没有写出点评")
	}
	if err := s.db.WithContext(ctx).Model(&reviewRow{}).Where("id = ?", row.ID).
		Updates(map[string]any{"content": content, "created_by": by, "updated_at": time.Now()}).Error; err != nil {
		return Review{}, err
	}
	row.Content, row.CreatedBy, row.UpdatedAt = content, by, time.Now()
	return row.toDTO(), nil
}

const reviewSystem = `你帮爸爸妈妈看孩子这一周的饮食：他们排了计划，家里人（多半是奶奶）记了每天实际吃了什么。
写一段对照点评，给排计划的爸爸妈妈看，帮他们排下周的菜。

要求：
- 先一句话总体：这周大体有没有照计划吃、吃得怎么样。
- 然后分几点（每点一两句，总共不超过 6 点）：
  - 吃得好的、明显爱吃的；
  - 没按计划的、剩得多的、总不肯吃的（青菜、肉、水果、奶……），说出是哪几天；
  - 计划本身的问题：某类东西太少或重复太多、要求没落实（比如「一天一杯奶」只记到两天）。
- 最后给下周 1-3 条具体建议（换个做法、换种菜、哪餐加什么）；下周已经排了的，就对着它说。
- 只根据给你的记录说话。没记的不等于没吃，记录明显不全时说一句「记录不全，仅供参考」，别下结论。
- 语气平和、就事论事。喂饭的人很辛苦，不评价谁做得好不好，不说「奶奶没有……」这类话，只说孩子吃了什么、没吃什么。
- 不替代医生：体重、过敏、长期不吃某类东西这类问题，建议问儿科医生。
- 用简单的 Markdown（加粗、列表），不要标题，不要客套话。`

// RunScheduler 每 15 分钟看一次，周日晚上 8 点以后给每个家庭的孩子生成这一周的点评并推送。
func (s *Service) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(reviewTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		now := time.Now().In(shanghai)
		if now.Weekday() != time.Sunday || now.Hour() < reviewHour {
			continue
		}
		var families []string
		if err := s.db.WithContext(ctx).Table("pa_family").Pluck("family_key", &families).Error; err != nil {
			continue
		}
		weekStart := weekStartOf(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local))
		for _, familyKey := range families {
			jobCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			review, err := s.review(jobCtx, familyKey, DefaultChild, weekStart, "", false)
			switch {
			case errors.Is(err, errAlreadyReviewed):
			case err != nil:
				log.Printf("饮食周点评（%s）: %v", familyKey, err)
			default:
				s.notify(jobCtx, familyKey, review)
			}
			cancel()
		}
	}
}

// notify 给每个能改计划的账号各建一条只提醒他自己的提醒，到点（马上）推到他的手机。
func (s *Service) notify(ctx context.Context, familyKey string, review Review) {
	if s.reminders == nil || s.planAccounts == nil {
		return
	}
	body := review.Content
	if runes := []rune(body); len(runes) > 120 {
		body = string(runes[:120]) + "…"
	}
	for _, account := range s.planAccounts(ctx) {
		userCtx := viewer.With(ctx, viewer.Viewer{Account: account})
		if _, err := s.reminders.CreateManual(userCtx, familyKey, reminder.ManualInput{
			Title: "这周宝宝吃饭的对照点评", Body: body, RemindAt: time.Now(), TargetURL: "/diet", Visibility: viewer.Private,
		}); err != nil {
			log.Printf("推送饮食周点评给 %s: %v", account, err)
		}
	}
}
