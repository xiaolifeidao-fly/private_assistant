package diet

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"common/keys"
	"common/middleware/db"
	"service/llm"
)

// 拍照 / 说一句 → 饮食记录（奶奶那边主要的记法）。
// 照片先存下来，再把照片、说的话、当天的计划一起交给模型整理成一条或几条记录，直接生效，不用再确认；
// 整理错了在列表里改。模型不可用或者没整理出来时，原话照样记一条，不丢。
const taskExtract = "diet.extract"

const (
	maxPhotos     = 4
	maxPhotoBytes = 5 << 20
)

var allowedPhotoTypes = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

// Chatter 是模型网关（service/llm.Gateway）。
type Chatter interface {
	Chat(ctx context.Context, request llm.Request, onDelta func(llm.Delta)) (llm.Response, error)
}

type Image struct {
	MediaType string
	Data      []byte
}

type CaptureInput struct {
	ChildKey string
	// Text 说的话（语音已经在 App 里转成文字）或打的字
	Text   string
	Images []Image
	// At 拍照/说话的时间，默认现在
	At *time.Time
}

type CaptureResult struct {
	Records []Record `json:"records"`
	// Reply 回给说话的人的一句话（长辈模式会读出来）
	Reply string `json:"reply"`
	// Fallback 为 true 表示没整理出来，原话原样记了一条
	Fallback bool `json:"fallback"`
}

type extracted struct {
	Records []struct {
		Meal    string `json:"meal"`
		EatenAt string `json:"eaten_at"`
		Foods   string `json:"foods"`
		Amount  string `json:"amount"`
		Note    string `json:"note"`
	} `json:"records"`
}

func extractSchema() map[string]any {
	meals := make([]string, 0, len(Meals))
	for _, meal := range Meals {
		meals = append(meals, meal.Key)
	}
	str := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	item := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"meal":     map[string]any{"type": "string", "enum": meals, "description": "哪一餐"},
			"eaten_at": str("什么时候吃的 YYYY-MM-DD HH:MM；话里没说、也看不出就填空串"),
			"foods":    str("实际吃了什么，写具体：菜名、主食、水果、奶，能看出分量就写上（米饭小半碗）"),
			"amount":   map[string]any{"type": "string", "enum": []string{"", "all", "most", "half", "little", "none"}, "description": "吃了多少：all 吃完 / most 大半 / half 一半 / little 几口 / none 没吃；不知道填空串"},
			"note":     str("情况：挑食、不肯吃、哭闹、谁喂的、加了什么零食、身体不舒服……没有填空串"),
		},
		"required": []string{"meal", "eaten_at", "foods", "amount", "note"},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{"records": map[string]any{"type": "array", "items": item}},
		"required":             []string{"records"},
	}
}

const extractSystem = `你帮一家人记孩子每天实际吃了什么。记的人多半是奶奶：她会拍一张饭菜的照片，或者说一句话（语音转的文字，可能有错别字、方言），也可能两样都有。
把这次说的、拍的整理成饮食记录，每一餐一条。

规则：
- 只记已经吃了的。说的是打算、计划（"明天给她做饺子"）、或者跟吃饭无关的，返回空的 records。
- 话里说的为准，照片用来补充：照片里看得出的菜写具体（番茄炒蛋、米饭、西兰花），碗里剩了多少能看出就写进 amount。
- 说"按计划吃的""都吃了"这类话，foods 写当天计划里那一餐的内容。
- 一句话里说了好几餐（"早上喝了奶，中午吃了面条"），分开记。
- 哪一餐：话里说了按话里的；没说就按时间判断。时间按"现在"换算（"刚才""中午"），说不出具体几点就留空。
- 不要编造没说、照片里也看不到的东西。语音转写的明显错字按意思改正（"西蓝花"写成"西兰花"可以，人名、菜名拿不准就照原样写）。
- foods 简短，用顿号隔开；note 写情况，不写套话。`

// Capture 拍照 / 说一句，整理成饮食记录。谁都能记。
func (s *Service) Capture(ctx context.Context, familyKey string, input CaptureInput, actor Actor) (CaptureResult, error) {
	childKey, childLabel, err := s.child(ctx, familyKey, input.ChildKey)
	if err != nil {
		return CaptureResult{}, err
	}
	text := clip(input.Text)
	if text == "" && len(input.Images) == 0 {
		return CaptureResult{}, errors.New("拍张照或者说一句吃了什么")
	}
	blocks, err := imageBlocks(input.Images)
	if err != nil {
		return CaptureResult{}, err
	}
	at := time.Now()
	if input.At != nil && !input.At.IsZero() && input.At.Before(at.Add(time.Minute)) {
		at = *input.At
	}
	captureKey := keys.New("dcap")
	var photos []Photo
	for index, image := range input.Images {
		path := fmt.Sprintf("diet/%s/%s/%d.%s", familyKey, captureKey, index, allowedPhotoTypes[image.MediaType])
		if err := s.storage.Save(path, image.Data); err != nil {
			return CaptureResult{}, fmt.Errorf("保存照片失败: %w", err)
		}
		photos = append(photos, Photo{Path: path, MediaType: image.MediaType, Size: len(image.Data)})
	}

	day, _ := parseDate(at.In(shanghai).Format(dateLayout))
	overview, err := s.Overview(ctx, familyKey, childKey, day.Format(dateLayout), 1, actor)
	if err != nil {
		return CaptureResult{}, err
	}
	seeds, extractErr := s.extract(ctx, captureKey, blocks, captureHeader(at, childLabel, text, len(photos), overview))
	fallback := false
	if extractErr != nil || len(seeds) == 0 {
		if extractErr != nil {
			log.Printf("饮食整理失败，原话照记: %v", extractErr)
		}
		// 没整理出来（模型不可用、说的不是吃了什么）：有照片或原话就照记一条，不丢；改一改就行
		fallback = true
		foods := text
		if foods == "" {
			foods = "（见照片）"
		}
		seeds = []RecordInput{{EatenAt: &at, Meal: GuessMeal(at.In(shanghai)), Foods: foods}}
	}

	now := time.Now()
	result := CaptureResult{Fallback: fallback}
	for _, seed := range seeds {
		seed.ChildKey = childKey
		if seed.EatenAt == nil {
			seed.EatenAt = &at
		}
		if err := seed.normalize(); err != nil {
			continue
		}
		row := recordRow{RecordKey: keys.New("drec"), FamilyKey: familyKey, ChildKey: childKey, EatenAt: *seed.EatenAt, Meal: seed.Meal,
			Foods: seed.Foods, Amount: seed.Amount, Note: seed.Note, Source: "capture", RawText: text, CaptureKey: captureKey,
			Status: statusActive, CreatedBy: actor.Account, UpdatedBy: actor.Account, CreatedAt: now, UpdatedAt: now}
		if len(photos) > 0 {
			row.Photos = db.MustJSON(photos)
		}
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return result, err
		}
		result.Records = append(result.Records, row.toDTO())
	}
	result.Reply = replyFor(result.Records, fallback)
	return result, nil
}

func (s *Service) extract(ctx context.Context, captureKey string, images []llm.Block, header string) ([]RecordInput, error) {
	var parsed extracted
	if err := s.askJSON(ctx, taskExtract, extractSystem, extractSchema(), images, header, captureKey, &parsed); err != nil {
		return nil, err
	}
	var seeds []RecordInput
	for _, item := range parsed.Records {
		seed := RecordInput{Meal: item.Meal, Foods: item.Foods, Amount: item.Amount, Note: item.Note}
		if parsed, err := time.ParseInLocation("2006-01-02 15:04", strings.TrimSpace(item.EatenAt), shanghai); err == nil && parsed.Before(time.Now().Add(time.Hour)) {
			seed.EatenAt = &parsed
		}
		seeds = append(seeds, seed)
	}
	return seeds, nil
}

// askJSON 把照片和文字交给模型，按 schema 解析出结果。
func (s *Service) askJSON(ctx context.Context, task, system string, schema map[string]any, images []llm.Block, text, refKey string, target any) error {
	if s.llm == nil {
		return errors.New("没有模型网关")
	}
	blocks := append(append([]llm.Block{}, images...), llm.Block{Type: llm.BlockText, Text: text})
	response, err := s.llm.Chat(ctx, llm.Request{
		Task:       task,
		System:     []llm.SystemPart{{Text: system, Cache: true}},
		Messages:   []llm.Message{{Role: llm.RoleUser, Blocks: blocks}},
		JSONSchema: schema,
		RefKey:     refKey,
	}, nil)
	if err != nil {
		return err
	}
	switch response.StopReason {
	case llm.StopRefusal:
		return errors.New("模型拒绝处理")
	case llm.StopMaxToken:
		return fmt.Errorf("输出被截断了，请少说几天，或在管理端调大 %s 的 max_tokens", task)
	}
	if err := json.Unmarshal([]byte(llm.JSONText(response.Message.Text())), target); err != nil {
		return fmt.Errorf("解析整理结果失败: %w", err)
	}
	return nil
}

// imageBlocks 校验照片格式大小，转成给模型看的图片块。
func imageBlocks(images []Image) ([]llm.Block, error) {
	if len(images) > maxPhotos {
		return nil, fmt.Errorf("一次最多 %d 张照片", maxPhotos)
	}
	var blocks []llm.Block
	for index, image := range images {
		if _, ok := allowedPhotoTypes[image.MediaType]; !ok {
			return nil, fmt.Errorf("第 %d 张照片格式不支持（%s）", index+1, image.MediaType)
		}
		if len(image.Data) > maxPhotoBytes {
			return nil, fmt.Errorf("第 %d 张照片超过 5MB", index+1)
		}
		blocks = append(blocks, llm.Block{Type: llm.BlockImage, MediaType: image.MediaType, Data: base64.StdEncoding.EncodeToString(image.Data)})
	}
	return blocks, nil
}

// captureHeader 是交给模型的上下文：现在几点、当天计划了什么、已经记了什么、这次说了什么。
func captureHeader(at time.Time, childLabel, text string, photoCount int, overview Overview) string {
	var builder strings.Builder
	local := at.In(shanghai)
	weekdays := []string{"日", "一", "二", "三", "四", "五", "六"}
	fmt.Fprintf(&builder, "现在：%s（周%s）\n孩子：%s\n", local.Format("2006-01-02 15:04"), weekdays[local.Weekday()], childLabel)
	builder.WriteString("\n今天的计划：\n")
	if len(overview.Plans) == 0 {
		builder.WriteString("（没有计划）\n")
	}
	for _, plan := range overview.Plans {
		fmt.Fprintf(&builder, "- %s：%s", MealLabel(plan.Meal), plan.Foods)
		if plan.Note != "" {
			fmt.Fprintf(&builder, "（要求：%s）", plan.Note)
		}
		builder.WriteString("\n")
	}
	if len(overview.Records) > 0 {
		builder.WriteString("\n今天已经记过的（不要重复记）：\n")
		for _, record := range overview.Records {
			fmt.Fprintf(&builder, "- %s %s：%s\n", record.EatenAt.In(shanghai).Format("15:04"), MealLabel(record.Meal), record.Foods)
		}
	}
	builder.WriteString("\n这次")
	if photoCount > 0 {
		fmt.Fprintf(&builder, "拍了 %d 张照片（在上面）", photoCount)
	}
	if text != "" {
		if photoCount > 0 {
			builder.WriteString("，")
		}
		fmt.Fprintf(&builder, "说的是：\n%s\n", text)
	} else {
		builder.WriteString("，没有说话。\n")
	}
	return builder.String()
}

// replyFor 回一句记了什么，按记下来的内容拼，不让模型自由发挥（长辈模式会读出来）。
func replyFor(records []Record, fallback bool) string {
	if len(records) == 0 {
		return "没记上，再说一遍？"
	}
	if fallback {
		return "原话先记下了，" + MealLabel(records[0].Meal) + "：" + records[0].Foods + "。不对的话点一下改。"
	}
	parts := make([]string, 0, len(records))
	for _, record := range records {
		part := MealLabel(record.Meal) + "：" + record.Foods
		if label, ok := Amounts[record.Amount]; ok {
			part += "，" + label
		}
		parts = append(parts, part)
	}
	return "记好了。" + strings.Join(parts, "；") + "。"
}
