package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBusyProviderRetriesThenSucceeds(t *testing.T) {
	saved := busyRetryDelays
	busyRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { busyRetryDelays = saved }()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(529)
			fmt.Fprint(w, `{"type":"error","error":{"type":"overloaded_error","message":"当前服务集群负载较高"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"好\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	route := Route{Kind: KindOpenAI, BaseURL: server.URL, Model: "m", MaxTokens: 10}
	response, err := (&Gateway{}).call(context.Background(), route, Request{Messages: []Message{{Role: RoleUser, Blocks: []Block{{Type: BlockText, Text: "hi"}}}}}, nil)
	if err != nil || calls != 3 || response.Message.Text() != "好" {
		t.Fatalf("calls=%d err=%v text=%q", calls, err, response.Message.Text())
	}
}

func TestBusyErrorReadsPlain(t *testing.T) {
	err := &ProviderError{Status: 529, Message: `{"type":"error"}`}
	if strings.Contains(err.Error(), "{") || !strings.Contains(err.Detail(), "overloaded") && !strings.Contains(err.Detail(), "529") {
		t.Fatalf("Error=%q Detail=%q", err.Error(), err.Detail())
	}
}
