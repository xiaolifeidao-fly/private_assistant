package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// MiniMax 的向量化接口（embo-01，1536 维）。新版文档里已经没有这个接口，但老接口仍然可用（2026-10-13 实测）：
//
//	POST {scheme}://{host}/v1/embeddings  {"model":"embo-01","texts":[...],"type":"db"|"query"}
//	→ {"vectors":[[...]], "base_resp":{"status_code":0,"status_msg":"success"}}
//
// 和 OpenAI 的区别：字段叫 texts / vectors；存入用 db、检索用 query，两种编码不能混用；
// 出错也返回 HTTP 200，错误码在 base_resp 里。

type embedPurpose string

const (
	embedDocument embedPurpose = "db"
	embedQuery    embedPurpose = "query"

	miniMaxEmbedBatch = 16
)

func isMiniMax(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	return err == nil && strings.Contains(parsed.Host, "minimax")
}

func embedMiniMax(ctx context.Context, route Route, texts []string, purpose embedPurpose) ([][]float32, error) {
	parsed, err := url.Parse(strings.TrimSpace(route.BaseURL))
	if err != nil {
		return nil, err
	}
	endpoint := parsed.Scheme + "://" + parsed.Host + "/v1/embeddings"
	result := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += miniMaxEmbedBatch {
		batch := texts[start:min(start+miniMaxEmbedBatch, len(texts))]
		vectors, err := postMiniMaxEmbedding(ctx, route, endpoint, batch, purpose)
		if err != nil {
			return nil, err
		}
		result = append(result, vectors...)
	}
	return result, nil
}

func postMiniMaxEmbedding(ctx context.Context, route Route, endpoint string, texts []string, purpose embedPurpose) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": route.Model, "texts": texts, "type": string(purpose)})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+route.APIKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("embedding 请求失败: %w", err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		return nil, &ProviderError{Status: response.StatusCode, Message: fmt.Sprintf("embedding 失败 %d: %s", response.StatusCode, truncate(string(data), 300))}
	}
	var payload struct {
		Vectors  [][]float32 `json:"vectors"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("embedding 结果解析失败: %s", truncate(string(data), 200))
	}
	if payload.BaseResp.StatusCode != 0 {
		return nil, &ProviderError{Status: response.StatusCode, Message: fmt.Sprintf("embedding 失败（%d）：%s", payload.BaseResp.StatusCode, payload.BaseResp.StatusMsg)}
	}
	if len(payload.Vectors) != len(texts) {
		return nil, fmt.Errorf("embedding 返回 %d 条，期望 %d 条", len(payload.Vectors), len(texts))
	}
	for index, vector := range payload.Vectors {
		payload.Vectors[index] = Normalize(vector)
	}
	return payload.Vectors, nil
}
