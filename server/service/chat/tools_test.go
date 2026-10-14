package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"service/bot"
)

func TestAskPersonaOnlyAssistantCanCall(t *testing.T) {
	_, err := runAskPersona(context.Background(), nil, toolScope{
		bot: bot.Bot{Kind: bot.KindPersona},
	}, json.RawMessage(`{"persona":"wife","message":"今晚回家吃饭吗"}`))
	if err == nil || !strings.Contains(err.Error(), "只有助理") {
		t.Fatalf("分身不应能调用另一个分身: %v", err)
	}
}

func TestAskPersonaOnlyAllowsAdultRehearsalBots(t *testing.T) {
	_, err := runAskPersona(context.Background(), nil, toolScope{
		bot: bot.Bot{Kind: bot.KindAssistant},
	}, json.RawMessage(`{"persona":"daughter","message":"今晚早点睡"}`))
	if err == nil || !strings.Contains(err.Error(), "老婆或妈妈") {
		t.Fatalf("女儿档案不应被当成成人分身预演: %v", err)
	}
}

func TestAskPersonaSchemaIsRestricted(t *testing.T) {
	persona := toolSpecs()[bot.ToolAskPersona].definition.Properties["persona"].(map[string]any)
	values, ok := persona["enum"].([]string)
	if !ok || strings.Join(values, ",") != "wife,mom" {
		t.Fatalf("ask_persona 的 persona 枚举错误: %#v", persona["enum"])
	}
}
