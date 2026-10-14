package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbedMiniMaxBatchesAndPurpose(t *testing.T) {
	var batches []int
	var types []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("路径或鉴权错误: %s", r.URL.Path)
		}
		var body struct {
			Model string   `json:"model"`
			Texts []string `json:"texts"`
			Type  string   `json:"type"`
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		batches = append(batches, len(body.Texts))
		types = append(types, body.Type)
		vectors := make([]string, len(body.Texts))
		for i := range vectors {
			vectors[i] = "[3,4]"
		}
		fmt.Fprintf(w, `{"vectors":[%s],"base_resp":{"status_code":0,"status_msg":"success"}}`, strings.Join(vectors, ","))
	}))
	defer server.Close()
	// 提供商地址写的是 Anthropic 兼容地址也没关系：只取 host
	route := Route{BaseURL: strings.Replace(server.URL, "127.0.0.1", "127.0.0.1", 1) + "/anthropic", APIKey: "k", Model: "embo-01"}
	texts := make([]string, 20)
	vectors, err := embedMiniMax(context.Background(), route, texts, embedDocument)
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 20 || len(batches) != 2 || batches[0] != 16 || types[0] != "db" {
		t.Fatalf("应分两批、用 db: %v %v", batches, types)
	}
	if vectors[0][0] != 0.6 || vectors[0][1] != 0.8 {
		t.Fatalf("应归一化: %v", vectors[0])
	}
	if _, err := embedMiniMax(context.Background(), route, []string{"q"}, embedQuery); err != nil || types[2] != "query" {
		t.Fatalf("检索用 query: %v %v", types, err)
	}
}

func TestEmbedMiniMaxBaseRespError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"vectors":null,"base_resp":{"status_code":1004,"status_msg":"authentication failed"}}`)
	}))
	defer server.Close()
	_, err := embedMiniMax(context.Background(), Route{BaseURL: server.URL, APIKey: "k", Model: "embo-01"}, []string{"x"}, embedDocument)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("HTTP 200 但 base_resp 报错时要返回错误: %v", err)
	}
	if !isMiniMax("https://api.minimax.cn/anthropic") || isMiniMax("https://dashscope.aliyuncs.com/compatible-mode/v1") {
		t.Fatal("isMiniMax 判断错误")
	}
}
