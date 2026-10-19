package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"common/keys"
	"service/memory"
	"service/wire"

	"gorm.io/gorm"
)

// fixture 是一份虚构的家庭数据。人物用迁移里的默认成员，这里只补档案和记忆。
type fixture struct {
	Persons  []fixturePerson `json:"persons"`
	Places   []fixturePlace  `json:"places"`
	Memories []fixtureMemory `json:"memories"`
	Notes    []fixtureNote   `json:"notes"`
}

// fixtureNote 是一条原始记录，内容故意没有整理成记忆：测「整理漏了的话，原文还能不能搜到」。
type fixtureNote struct {
	Ref        string   `json:"ref"` // 用例里用 #ref 引用
	Channel    string   `json:"channel"`
	Text       string   `json:"text"`
	About      []string `json:"about"`
	OccurredAt string   `json:"occurredAt"`
}

type fixturePerson struct {
	Key     string `json:"key"`
	Profile string `json:"profile"`
}

type fixturePlace struct {
	Key        string `json:"key"`
	Customs    string `json:"customs"`
	TravelTips string `json:"travelTips"`
}

type fixtureMemory struct {
	Ref        string          `json:"ref"` // 用例里用 @ref 引用
	Type       string          `json:"type"`
	Subject    string          `json:"subject"`
	Persons    []string        `json:"persons"`
	Place      string          `json:"place"`
	Title      string          `json:"title"`
	Content    string          `json:"content"`
	Importance int             `json:"importance"`
	OccurredAt string          `json:"occurredAt"` // YYYY-MM-DD，或 -3d 表示三天前
	Attrs      json.RawMessage `json:"attrs"`
}

// 每次评测前清空的表：记忆、对话、人设卡、提醒；机器人和手册重置成代码里的出厂内容，
// 评测的就是仓库里的提示词和手册。
var resetTables = []string{
	"pa_memory", "pa_memory_link", "pa_memory_embedding", "pa_conversation", "pa_message",
	"pa_persona_card", "pa_reminder", "pa_bot", "pa_skill", "pa_source_note",
}

func resetAndSeed(ctx context.Context, database *gorm.DB, services *wire.Services, options wire.Options) (map[string]string, error) {
	// 带上 ctx（评测是 Unscoped）：不声明身份的语句会被可见范围插件拒绝
	database = database.WithContext(ctx)
	for _, table := range resetTables {
		if err := database.Exec("DELETE FROM " + table).Error; err != nil {
			return nil, fmt.Errorf("清空 %s: %w", table, err)
		}
	}
	services.Seed(options)

	content, err := os.ReadFile("fixture.json")
	if err != nil {
		return nil, err
	}
	var data fixture
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("fixture.json: %w", err)
	}
	for _, person := range data.Persons {
		if err := database.Exec("UPDATE pa_person SET profile = ? WHERE family_key = ? AND person_key = ?", person.Profile, wire.DefaultFamily, person.Key).Error; err != nil {
			return nil, err
		}
	}
	for _, place := range data.Places {
		if err := database.Exec("UPDATE pa_place SET customs = ?, travel_tips = ? WHERE family_key = ? AND place_key = ?",
			place.Customs, place.TravelTips, wire.DefaultFamily, place.Key).Error; err != nil {
			return nil, err
		}
	}

	refs := map[string]string{}
	for _, item := range data.Memories {
		occurred, err := fixtureTime(item.OccurredAt)
		if err != nil {
			return nil, fmt.Errorf("记忆 %s 的时间: %w", item.Ref, err)
		}
		attrs := item.Attrs
		if len(attrs) == 0 {
			attrs = nil
		}
		created, err := services.Memory.Create(ctx, wire.DefaultFamily, memory.Input{
			Type: item.Type, SubjectPerson: item.Subject, Persons: item.Persons, PlaceKey: item.Place,
			Title: item.Title, Content: item.Content, Attrs: attrs, Importance: item.Importance,
			Confidence: 1, Status: memory.StatusActive, OccurredAt: occurred, CreatedBy: "eval",
		})
		if err != nil {
			return nil, fmt.Errorf("记忆 %s: %w", item.Ref, err)
		}
		if item.Ref != "" {
			refs[item.Ref] = created.MemoryKey
			refs["@"+item.Ref] = created.MemoryKey
		}
	}
	for _, note := range data.Notes {
		occurred, err := fixtureTime(note.OccurredAt)
		if err != nil {
			return nil, fmt.Errorf("原始记录 %s 的时间: %w", note.Ref, err)
		}
		about, _ := json.Marshal(note.About)
		key := keys.New("note")
		// 标成已整理：评测不跑后台 worker，也不希望它被整理成记忆
		if err := database.Exec(`INSERT INTO pa_source_note (note_key, family_key, channel, raw_text, about_persons, occurred_at, extract_status, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'done', 'eval', ?)`, key, wire.DefaultFamily, note.Channel, note.Text, string(about), occurred, time.Now()).Error; err != nil {
			return nil, fmt.Errorf("原始记录 %s: %w", note.Ref, err)
		}
		refs["#"+note.Ref] = key
	}
	// 向量是异步算的，等它算完再跑，结果才接近线上
	// 没配向量化时是空操作
	if count, err := services.Memory.ReindexMissing(ctx, wire.DefaultFamily); err != nil {
		log.Printf("补算向量失败（检索会退化为全文）: %v", err)
	} else if count > 0 {
		log.Printf("补算了 %d 条向量", count)
	}
	if count, err := services.Memory.ReindexPassages(ctx, wire.DefaultFamily); err != nil {
		log.Printf("补算原文片段向量失败: %v", err)
	} else if count > 0 {
		log.Printf("补算了 %d 段原文向量", count)
	}
	log.Printf("装好 %d 条记忆、%d 条原始记录", len(data.Memories), len(data.Notes))
	return refs, nil
}

func fixtureTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	if value[0] == '-' {
		duration, err := time.ParseDuration(value[1:len(value)-1] + "h")
		if err != nil || value[len(value)-1] != 'd' {
			return nil, fmt.Errorf("相对时间只支持 -Nd: %s", value)
		}
		at := time.Now().Add(-duration * 24)
		return &at, nil
	}
	at, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil, err
	}
	return &at, nil
}
