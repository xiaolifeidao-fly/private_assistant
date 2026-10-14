package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// writeReport 写一份 Markdown 报告到 reports/（不进 git，里面有完整对话）。
func writeReport(results []result, at time.Time, configLabel string) (string, error) {
	var builder strings.Builder
	passed, tokens := 0, 0
	for _, outcome := range results {
		if outcome.passed() {
			passed++
		}
		tokens += outcome.Tokens
	}
	fmt.Fprintf(&builder, "# 评测报告 %s\n\n通过 **%d / %d**，共消耗约 %d token。评测的配置：%s。\n\n", at.Format("2006-01-02 15:04"), passed, len(results), tokens, configLabel)
	builder.WriteString("| 用例 | 结果 | 工具 | 手册 | token |\n|---|---|---|---|---|\n")
	for _, outcome := range results {
		status := "✅"
		if !outcome.passed() {
			status = fmt.Sprintf("❌ %d 项", len(outcome.Failures))
		}
		fmt.Fprintf(&builder, "| %s | %s | %s | %s | %d |\n", outcome.Label, status,
			strings.Join(outcome.Tools, ", "), strings.Join(outcome.Skills, ", "), outcome.Tokens)
	}
	for _, outcome := range results {
		fmt.Fprintf(&builder, "\n---\n\n## %s\n\n`%s` · 机器人 `%s`\n\n", outcome.Label, outcome.Case.File, outcome.Case.Bot)
		if len(outcome.Failures) > 0 {
			builder.WriteString("**未通过**\n\n")
			for _, failure := range outcome.Failures {
				builder.WriteString("- " + failure + "\n")
			}
			builder.WriteString("\n")
		}
		if len(outcome.Scores) > 0 {
			builder.WriteString("**打分**\n\n")
			for _, score := range outcome.Scores {
				mark := "✅"
				if !score.Pass {
					mark = "❌"
				}
				fmt.Fprintf(&builder, "- %s %s —— %s\n", mark, score.Rubric, score.Reason)
			}
			builder.WriteString("\n")
		}
		builder.WriteString("<details><summary>对话</summary>\n\n```\n" + outcome.Transcript + "```\n\n</details>\n")
	}
	if err := os.MkdirAll("reports", 0o755); err != nil {
		return "", err
	}
	path := filepath.Join("reports", at.Format("20060102-150405")+".md")
	return path, os.WriteFile(path, []byte(builder.String()), 0o644)
}
