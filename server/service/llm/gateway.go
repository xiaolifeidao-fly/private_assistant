package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"common/keys"

	"gorm.io/gorm"
)

const TaskEmbedding = "embedding"

var ErrNoEmbedding = errors.New("没有配置可用的 embedding 提供商")

type Gateway struct{ db *gorm.DB }

func New(database *gorm.DB) *Gateway { return &Gateway{db: database} }

// Chat 按 request.Task 路由并调用模型，流式增量通过 onDelta 回调。
func (g *Gateway) Chat(ctx context.Context, request Request, onDelta func(Delta)) (Response, error) {
	route, err := g.Resolve(ctx, request.Task)
	if err != nil {
		return Response{}, err
	}
	response, err := g.call(ctx, route, request, onDelta)
	if err == nil && request.JSONSchema != nil && !nativeJSON(route) && response.StopReason != StopMaxToken {
		response, err = g.repairJSON(ctx, route, request, response)
	}
	return response, err
}

func (g *Gateway) call(ctx context.Context, route Route, request Request, onDelta func(Delta)) (Response, error) {
	started := time.Now()
	var response Response
	var err error
	switch route.Kind {
	case KindAnthropic:
		response, err = chatAnthropic(ctx, route, request, onDelta)
	case KindOpenAI:
		response, err = chatOpenAI(ctx, route, request, onDelta)
	default:
		err = fmt.Errorf("不支持的提供商类型 %s", route.Kind)
	}
	g.logCall(route, request.RefKey, response, err, time.Since(started))
	return response, err
}

// nativeJSON 表示模型本身保证输出合法 JSON（Claude 的结构化输出）。
func nativeJSON(route Route) bool { return route.Kind == KindAnthropic && isClaude(route) }

// repairJSON：不支持结构化输出的模型只靠提示词约束，偶尔会输出不合法的 JSON
// （比如字符串里的双引号没转义）。把解析错误告诉模型，让它重新输出一次。
func (g *Gateway) repairJSON(ctx context.Context, route Route, request Request, response Response) (Response, error) {
	text := JSONText(response.Message.Text())
	var probe any
	parseErr := json.Unmarshal([]byte(text), &probe)
	if parseErr == nil {
		return response, nil
	}
	retry := request
	retry.Messages = append(append([]Message{}, request.Messages...),
		Message{Role: RoleAssistant, Blocks: []Block{{Type: BlockText, Text: response.Message.Text()}}},
		Message{Role: RoleUser, Blocks: []Block{{Type: BlockText, Text: fmt.Sprintf(
			"你上面的输出不是合法的 JSON（%v）。请只输出修正后的完整 JSON，不要任何解释；字符串里的双引号用 \\\" 转义，或改用中文引号「」。", parseErr)}}})
	fixed, err := g.call(ctx, route, retry, nil)
	if err != nil {
		return response, nil // 修复失败就交回原输出，由调用方报解析错误
	}
	fixed.Usage.InputTokens += response.Usage.InputTokens
	fixed.Usage.OutputTokens += response.Usage.OutputTokens
	return fixed, nil
}

// JSONText 从模型输出里取出 JSON 部分：容忍 ```json 代码块和前后的说明文字。
func JSONText(text string) string {
	text = strings.TrimSpace(text)
	if start := strings.Index(text, "{"); start > 0 {
		text = text[start:]
	}
	if end := strings.LastIndex(text, "}"); end >= 0 && end < len(text)-1 {
		text = text[:end+1]
	}
	return text
}

// Embed 把要存起来的文本转成单位向量，返回所用模型名（向量必须和模型一起存）。
func (g *Gateway) Embed(ctx context.Context, texts []string) ([][]float32, string, error) {
	return g.embed(ctx, texts, embedDocument)
}

// EmbedQuery 把检索时的问题转成向量。有的模型（MiniMax embo-01）对存入和查询用不同的编码方式。
func (g *Gateway) EmbedQuery(ctx context.Context, text string) ([]float32, string, error) {
	vectors, model, err := g.embed(ctx, []string{text}, embedQuery)
	if err != nil {
		return nil, model, err
	}
	return vectors[0], model, nil
}

func (g *Gateway) embed(ctx context.Context, texts []string, purpose embedPurpose) ([][]float32, string, error) {
	route, err := g.Resolve(ctx, TaskEmbedding)
	if err != nil {
		return nil, "", ErrNoEmbedding
	}
	started := time.Now()
	var vectors [][]float32
	switch {
	case isMiniMax(route.BaseURL):
		vectors, err = embedMiniMax(ctx, route, texts, purpose)
	case route.Kind == KindOpenAI:
		vectors, err = embedOpenAI(ctx, route, texts)
	default:
		err = fmt.Errorf("embedding 只支持 OpenAI 兼容协议或 MiniMax 的提供商")
	}
	g.logCall(route, "", Response{}, err, time.Since(started))
	return vectors, route.Model, err
}

// EmbeddingModel 返回当前 embedding 路由的模型名；未配置返回空串。
func (g *Gateway) EmbeddingModel(ctx context.Context) string {
	route, err := g.Resolve(ctx, TaskEmbedding)
	if err != nil {
		return ""
	}
	return route.Model
}

// Resolve 把任务解析成可直接调用的路由配置。
func (g *Gateway) Resolve(ctx context.Context, task string) (Route, error) {
	var route routeRow
	if err := g.db.WithContext(ctx).Where("task = ?", task).First(&route).Error; err != nil {
		return Route{}, fmt.Errorf("没有配置任务 %s 的模型路由", task)
	}
	var provider providerRow
	if err := g.db.WithContext(ctx).Where("provider_key = ?", route.ProviderKey).First(&provider).Error; err != nil {
		return Route{}, fmt.Errorf("任务 %s 指向的提供商 %s 不存在，请到管理端「模型」里添加", task, route.ProviderKey)
	}
	if provider.Enabled != 1 {
		return Route{}, fmt.Errorf("提供商 %s 已停用", provider.ProviderKey)
	}
	if provider.Kind == KindAnthropic && provider.APIKey == "" {
		return Route{}, fmt.Errorf("提供商 %s 没有配置 API Key", provider.ProviderKey)
	}
	maxTokens := route.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 16000
	}
	return Route{
		Task: task, ProviderKey: provider.ProviderKey, Kind: provider.Kind, BaseURL: strings.TrimSpace(provider.BaseURL),
		APIKey: provider.APIKey, Model: route.Model, MaxTokens: maxTokens, Effort: route.Effort, Temperature: route.Temperature,
	}, nil
}

func (g *Gateway) logCall(route Route, refKey string, response Response, callErr error, elapsed time.Duration) {
	if g.db == nil {
		return
	}
	row := callRow{
		CallKey: keys.New("call"), Task: route.Task, ProviderKey: route.ProviderKey, Model: route.Model, RefKey: refKey,
		InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens,
		CacheReadTokens: response.Usage.CacheReadTokens, CacheWriteTokens: response.Usage.CacheWriteTokens,
		LatencyMS: int(elapsed.Milliseconds()), StopReason: response.StopReason, Status: "ok", CreatedAt: time.Now(),
	}
	if callErr != nil {
		row.Status = "error"
		row.Error = truncate(callErr.Error(), 1000)
	}
	// 记账失败不影响主流程。
	_ = g.db.Create(&row).Error
}

// ---------------------------------------------------------------- 管理

type Provider struct {
	ProviderKey string `json:"providerKey"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	BaseURL     string `json:"baseUrl"`
	APIKey      string `json:"apiKey,omitempty"` // 只写；读出时是掩码
	HasAPIKey   bool   `json:"hasApiKey"`
	Enabled     bool   `json:"enabled"`
}

func (g *Gateway) ListProviders(ctx context.Context) ([]Provider, error) {
	var rows []providerRow
	if err := g.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Provider, 0, len(rows))
	for _, row := range rows {
		result = append(result, Provider{
			ProviderKey: row.ProviderKey, Name: row.Name, Kind: row.Kind, BaseURL: row.BaseURL,
			APIKey: maskKey(row.APIKey), HasAPIKey: row.APIKey != "", Enabled: row.Enabled == 1,
		})
	}
	return result, nil
}

// SaveProvider 新增或更新提供商。APIKey 留空表示不改。
func (g *Gateway) SaveProvider(ctx context.Context, input Provider) error {
	input.ProviderKey = strings.TrimSpace(input.ProviderKey)
	if input.ProviderKey == "" || strings.TrimSpace(input.Name) == "" {
		return errors.New("提供商标识和名称不能为空")
	}
	if input.Kind != KindAnthropic && input.Kind != KindOpenAI {
		return errors.New("类型只能是 anthropic 或 openai")
	}
	enabled := 0
	if input.Enabled {
		enabled = 1
	}
	now := time.Now()
	var existing providerRow
	err := g.db.WithContext(ctx).Where("provider_key = ?", input.ProviderKey).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return g.db.WithContext(ctx).Create(&providerRow{
			ProviderKey: input.ProviderKey, Name: input.Name, Kind: input.Kind, BaseURL: strings.TrimSpace(input.BaseURL),
			APIKey: strings.TrimSpace(input.APIKey), Enabled: enabled, CreatedAt: now, UpdatedAt: now,
		}).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]any{"name": input.Name, "kind": input.Kind, "base_url": strings.TrimSpace(input.BaseURL), "enabled": enabled, "updated_at": now}
	if key := strings.TrimSpace(input.APIKey); key != "" && !strings.Contains(key, "****") {
		updates["api_key"] = key
	}
	return g.db.WithContext(ctx).Model(&existing).Updates(updates).Error
}

func (g *Gateway) DeleteProvider(ctx context.Context, providerKey string) error {
	var count int64
	g.db.WithContext(ctx).Model(&routeRow{}).Where("provider_key = ?", providerKey).Count(&count)
	if count > 0 {
		return errors.New("还有模型路由在用这个提供商")
	}
	return g.db.WithContext(ctx).Where("provider_key = ?", providerKey).Delete(&providerRow{}).Error
}

// EnsureProvider 在提供商不存在时用配置文件里的值创建（只创建，不覆盖管理端改过的配置）。
func (g *Gateway) EnsureProvider(ctx context.Context, input Provider) error {
	if strings.TrimSpace(input.APIKey) == "" && input.Kind == KindAnthropic {
		return nil
	}
	var count int64
	g.db.WithContext(ctx).Model(&providerRow{}).Where("provider_key = ?", input.ProviderKey).Count(&count)
	if count > 0 {
		return nil
	}
	input.Enabled = true
	return g.SaveProvider(ctx, input)
}

type RouteConfig struct {
	Task        string   `json:"task"`
	ProviderKey string   `json:"providerKey"`
	Model       string   `json:"model"`
	MaxTokens   int      `json:"maxTokens"`
	Effort      string   `json:"effort"`
	Temperature *float64 `json:"temperature"`
	Description string   `json:"description"`
}

func (g *Gateway) ListRoutes(ctx context.Context) ([]RouteConfig, error) {
	var rows []routeRow
	if err := g.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]RouteConfig, 0, len(rows))
	for _, row := range rows {
		result = append(result, RouteConfig{
			Task: row.Task, ProviderKey: row.ProviderKey, Model: row.Model, MaxTokens: row.MaxTokens,
			Effort: row.Effort, Temperature: row.Temperature, Description: row.Description,
		})
	}
	return result, nil
}

func (g *Gateway) SaveRoute(ctx context.Context, input RouteConfig) error {
	if input.Task == "" || input.ProviderKey == "" || input.Model == "" {
		return errors.New("任务、提供商、模型不能为空")
	}
	effort := strings.TrimSpace(input.Effort)
	switch effort {
	case "", "low", "medium", "high", "xhigh", "max":
	default:
		return errors.New("effort 只能是 low / medium / high / xhigh / max")
	}
	row := routeRow{
		Task: input.Task, ProviderKey: input.ProviderKey, Model: strings.TrimSpace(input.Model), MaxTokens: input.MaxTokens,
		Effort: effort, Temperature: input.Temperature, Description: input.Description, UpdatedAt: time.Now(),
	}
	var existing routeRow
	if err := g.db.WithContext(ctx).Where("task = ?", input.Task).First(&existing).Error; err == nil {
		return g.db.WithContext(ctx).Model(&existing).
			Select("provider_key", "model", "max_tokens", "effort", "temperature", "description", "updated_at").Updates(&row).Error
	}
	return g.db.WithContext(ctx).Create(&row).Error
}

type Call struct {
	CallKey          string    `json:"callKey"`
	Task             string    `json:"task"`
	ProviderKey      string    `json:"providerKey"`
	Model            string    `json:"model"`
	RefKey           string    `json:"refKey"`
	InputTokens      int       `json:"inputTokens"`
	OutputTokens     int       `json:"outputTokens"`
	CacheReadTokens  int       `json:"cacheReadTokens"`
	CacheWriteTokens int       `json:"cacheWriteTokens"`
	LatencyMS        int       `json:"latencyMs"`
	StopReason       string    `json:"stopReason"`
	Status           string    `json:"status"`
	Error            string    `json:"error"`
	CreatedAt        time.Time `json:"createdAt"`
}

func (g *Gateway) ListCalls(ctx context.Context, page, size int) ([]Call, int64, error) {
	if size <= 0 || size > 200 {
		size = 50
	}
	if page <= 0 {
		page = 1
	}
	var total int64
	g.db.WithContext(ctx).Model(&callRow{}).Count(&total)
	var rows []callRow
	if err := g.db.WithContext(ctx).Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	result := make([]Call, 0, len(rows))
	for _, row := range rows {
		result = append(result, Call{
			CallKey: row.CallKey, Task: row.Task, ProviderKey: row.ProviderKey, Model: row.Model, RefKey: row.RefKey,
			InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, CacheReadTokens: row.CacheReadTokens,
			CacheWriteTokens: row.CacheWriteTokens, LatencyMS: row.LatencyMS, StopReason: row.StopReason,
			Status: row.Status, Error: row.Error, CreatedAt: row.CreatedAt,
		})
	}
	return result, total, nil
}

// TestRoute 用一句话试调某个任务的路由，管理端「测试」按钮用。
func (g *Gateway) TestRoute(ctx context.Context, task string) (string, error) {
	if task == TaskEmbedding {
		vectors, model, err := g.Embed(ctx, []string{"测试"})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s 返回 %d 维向量", model, len(vectors[0])), nil
	}
	if task == TaskASR {
		// 一秒静音：能通就返回空文字，Key、地址、模型名错了会报错
		text, err := g.Transcribe(ctx, silentWAV(), "test.wav")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("语音识别连接正常（静音测试，识别结果：%q）", text), nil
	}
	response, err := g.Chat(ctx, Request{
		Task:     task,
		System:   []SystemPart{{Text: "你是连通性测试。"}},
		Messages: []Message{{Role: RoleUser, Blocks: []Block{{Type: BlockText, Text: "只回复：连接正常"}}}},
	}, nil)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("[%s] %s", response.Model, response.Message.Text()), nil
}

func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 10 {
		return "****"
	}
	return key[:6] + "****" + key[len(key)-4:]
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

// ---------------------------------------------------------------- 实体

type providerRow struct {
	ID          int64
	ProviderKey string
	Name        string
	Kind        string
	BaseURL     string
	APIKey      string
	Enabled     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (providerRow) TableName() string { return "pa_llm_provider" }

type routeRow struct {
	ID          int64
	Task        string
	ProviderKey string
	Model       string
	MaxTokens   int
	Effort      string
	Temperature *float64
	Description string
	UpdatedAt   time.Time
}

func (routeRow) TableName() string { return "pa_llm_route" }

type callRow struct {
	ID               int64
	CallKey          string
	Task             string
	ProviderKey      string
	Model            string
	RefKey           string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	LatencyMS        int `gorm:"column:latency_ms"`
	StopReason       string
	Status           string
	Error            string
	CreatedAt        time.Time
}

func (callRow) TableName() string { return "pa_llm_call" }
