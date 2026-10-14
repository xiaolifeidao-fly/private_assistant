package intake

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// 批量导入（设计文档 06 · M5）：把一大段导出的聊天记录按行切成若干块，每块一条原始记录，
// 走和速记一样的抽取管线。块不能太大：抽取模型一次看得太多容易漏。

const (
	importChunkRunes = 6000
	importMaxRunes   = 300000
)

type ImportInput struct {
	Text         string
	AboutPersons []string
	Note         string // 一句说明，比如「和老婆的微信，2025 年」，会放在每块开头
	CreatedBy    string
}

// ImportText 切块并排进抽取队列，返回建了多少条原始记录。
func (s *Service) ImportText(ctx context.Context, familyKey string, input ImportInput) (int, error) {
	text := strings.TrimSpace(strings.ReplaceAll(input.Text, "\r\n", "\n"))
	if text == "" {
		return 0, errors.New("内容不能为空")
	}
	if len([]rune(text)) > importMaxRunes {
		return 0, fmt.Errorf("一次最多导入 %d 万字，请分几次", importMaxRunes/10000)
	}
	chunks := splitChunks(text, importChunkRunes)
	for index, chunk := range chunks {
		header := fmt.Sprintf("【批量导入 第 %d/%d 段】", index+1, len(chunks))
		if note := strings.TrimSpace(input.Note); note != "" {
			header += note
		}
		if _, err := s.CreateNote(ctx, familyKey, NoteInput{
			Channel: "import", Text: header + "\n" + chunk, AboutPersons: input.AboutPersons, CreatedBy: defaultString(input.CreatedBy, "import"),
		}); err != nil {
			return index, fmt.Errorf("第 %d 段: %w", index+1, err)
		}
	}
	return len(chunks), nil
}

// splitChunks 按行切块，每块不超过 limit 个字；单行超长时硬切。
func splitChunks(text string, limit int) []string {
	var chunks []string
	var current []string
	size := 0
	flush := func() {
		if len(current) > 0 {
			chunks = append(chunks, strings.Join(current, "\n"))
			current, size = nil, 0
		}
	}
	for _, line := range strings.Split(text, "\n") {
		runes := []rune(line)
		for len(runes) > limit {
			flush()
			chunks = append(chunks, string(runes[:limit]))
			runes = runes[limit:]
		}
		if size+len(runes)+1 > limit {
			flush()
		}
		current = append(current, string(runes))
		size += len(runes) + 1
	}
	flush()
	return chunks
}
