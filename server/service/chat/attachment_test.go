package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"service/memory"
)

func TestFormatMemoryHidesPhotoRefs(t *testing.T) {
	item := memory.Memory{
		MemoryKey: "mem_1", Type: "item", TypeLabel: "物品", Title: "护照 → 书房抽屉", Content: "两本护照，深红色封皮",
		Attrs:      json.RawMessage(`{"location":"书房 / 书桌右边抽屉","photos":[{"note":"note_a","index":0},{"note":"note_a","index":1}]}`),
		Importance: 4, Status: memory.StatusActive, CreatedAt: time.Now(),
	}
	line := formatMemory(directory{}, item)
	if strings.Contains(line, "note_a") {
		t.Fatalf("照片引用不该给模型看: %s", line)
	}
	if !strings.Contains(line, "书桌右边抽屉") || !strings.Contains(line, "有 2 张照片") {
		t.Fatalf("应带位置和照片数: %s", line)
	}
	if attrsWithoutPhotos(json.RawMessage(`{"photos":[{"note":"n","index":0}]}`)) != "" {
		t.Fatal("只有照片时 attrs 应为空")
	}
}

func TestLearnerSkipsPhotoMessages(t *testing.T) {
	rows := []messageRow{
		{Role: "user", Content: "这是电钻，放阳台柜子下层", Meta: turnMeta{Attachment: &attachmentMeta{NoteKey: "note_a", Photos: 1}}.json()},
		{Role: "user", Content: "她昨天说想去杭州"},
	}
	text, mine := renderForLearning(rows, false)
	if strings.Contains(text, "电钻") || mine != len([]rune("她昨天说想去杭州")) {
		t.Fatalf("带照片的话已单独整理，不该再交给对话学习: %q %d", text, mine)
	}
}
