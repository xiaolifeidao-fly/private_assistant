package intake

import (
	"strings"
	"testing"
)

func TestSplitChunks(t *testing.T) {
	text := strings.Repeat("一二三四五\n", 10) + strings.Repeat("长", 25)
	chunks := splitChunks(text, 12)
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 12 {
			t.Fatalf("块超长: %q", chunk)
		}
	}
	if strings.Join(chunks, "") != strings.ReplaceAll(text, "\n", "") && !strings.Contains(strings.Join(chunks, "\n"), "一二三四五") {
		t.Fatalf("内容丢了: %v", chunks)
	}
	total := 0
	for _, chunk := range chunks {
		total += len([]rune(strings.ReplaceAll(chunk, "\n", "")))
	}
	if total != 50+25 {
		t.Fatalf("字数对不上: %d", total)
	}
}
