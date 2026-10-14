package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"service/wire"
)

// 检索评测：不让模型回答，只看「每轮自动检索」（chat.Retrieve）把该找到的记忆排在第几。
// 衡量的是命中率，用来比较检索改动前后的效果。
//
//	cd server/eval && go run . -retrieval [-case 充电]

type retrievalSuite struct {
	Bot   string          `json:"bot"`
	Cases []retrievalCase `json:"cases"`
}

type retrievalCase struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	History  []string `json:"history"`
	Query    string   `json:"query"`
	// Expect：命中其中任意一条就算找到（@记忆别名、#原始记录别名）；为空表示记忆里没有答案，什么都不该放进来。
	Expect []string `json:"expect"`
}

type retrievalOutcome struct {
	Case     retrievalCase
	Rank     int // 第一条命中的名次，从 1 开始；0 表示没找到
	Injected int // 进上下文的条数
	Top      []string
	Elapsed  time.Duration
	Error    string
}

func runRetrieval(ctx context.Context, services *wire.Services, refs map[string]string, filter string) ([]retrievalOutcome, error) {
	content, err := os.ReadFile(filepath.Join("retrieval", "cases.json"))
	if err != nil {
		return nil, err
	}
	var suite retrievalSuite
	if err := json.Unmarshal(content, &suite); err != nil {
		return nil, fmt.Errorf("retrieval/cases.json: %w", err)
	}
	titles := map[string]string{}
	for ref, key := range refs {
		titles[key] = ref
	}
	var outcomes []retrievalOutcome
	for _, item := range suite.Cases {
		if filter != "" && !strings.Contains(item.Name, filter) && !strings.Contains(item.Category, filter) {
			continue
		}
		expected := map[string]bool{}
		for _, ref := range item.Expect {
			key, ok := refs[ref]
			if !ok {
				return nil, fmt.Errorf("用例 %s 引用了不存在的别名 %s", item.Name, ref)
			}
			expected[key] = true
		}
		started := time.Now()
		results, err := services.Chat.Retrieve(ctx, wire.DefaultFamily, suite.Bot, item.History, item.Query)
		outcome := retrievalOutcome{Case: item, Elapsed: time.Since(started)}
		if err != nil {
			outcome.Error = err.Error()
		}
		outcome.Injected = len(results)
		for index, result := range results {
			label := titles[result.Key]
			if label == "" {
				label = result.Key
			}
			if index < 5 {
				outcome.Top = append(outcome.Top, fmt.Sprintf("%s(总%.2f 向%.2f 文%.2f)", strings.TrimLeft(label, "@#"), result.Score, result.Vector, result.Text))
			}
			if outcome.Rank == 0 && expected[result.Key] {
				outcome.Rank = index + 1
			}
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

type retrievalStats struct {
	Cases, Hit1, Hit3, HitAll int
	MRR                       float64
	NoneCases, NoneInjected   int
	Injected                  int
}

func (stats *retrievalStats) add(outcome retrievalOutcome) {
	stats.Injected += outcome.Injected
	if len(outcome.Case.Expect) == 0 {
		stats.NoneCases++
		stats.NoneInjected += outcome.Injected
		return
	}
	stats.Cases++
	if outcome.Rank > 0 {
		stats.HitAll++
		stats.MRR += 1 / float64(outcome.Rank)
		if outcome.Rank <= 3 {
			stats.Hit3++
		}
		if outcome.Rank == 1 {
			stats.Hit1++
		}
	}
}

func percent(part, total int) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(part)/float64(total))
}

func (stats retrievalStats) row(label string) string {
	mrr := "-"
	if stats.Cases > 0 {
		mrr = fmt.Sprintf("%.2f", stats.MRR/float64(stats.Cases))
	}
	none := "-"
	if stats.NoneCases > 0 {
		none = fmt.Sprintf("%.1f", float64(stats.NoneInjected)/float64(stats.NoneCases))
	}
	total := stats.Cases + stats.NoneCases
	avg := "-"
	if total > 0 {
		avg = fmt.Sprintf("%.1f", float64(stats.Injected)/float64(total))
	}
	return fmt.Sprintf("| %s | %d | %s | %s | %s | %s | %s | %s |", label, total,
		percent(stats.Hit1, stats.Cases), percent(stats.Hit3, stats.Cases), percent(stats.HitAll, stats.Cases), mrr, none, avg)
}

func writeRetrievalReport(outcomes []retrievalOutcome, label string) (string, string, error) {
	var total retrievalStats
	byCategory := map[string]*retrievalStats{}
	var categories []string
	for _, outcome := range outcomes {
		total.add(outcome)
		category := outcome.Case.Category
		if byCategory[category] == nil {
			byCategory[category] = &retrievalStats{}
			categories = append(categories, category)
		}
		byCategory[category].add(outcome)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# 检索评测 · %s\n\n%s\n\n", time.Now().Format("2006-01-02 15:04"), label)
	builder.WriteString("| 类别 | 用例 | 第 1 名命中 | 前 3 命中 | 进上下文 | MRR | 无答案时塞进的条数 | 平均塞进条数 |\n|---|---|---|---|---|---|---|---|\n")
	summary := total.row("**全部**")
	builder.WriteString(summary + "\n")
	for _, category := range categories {
		builder.WriteString(byCategory[category].row(category) + "\n")
	}
	builder.WriteString("\n## 逐条\n\n| 类别 | 问题 | 名次 | 前 5 | 耗时 |\n|---|---|---|---|---|\n")
	sort.SliceStable(outcomes, func(i, j int) bool { return outcomes[i].Case.Category < outcomes[j].Case.Category })
	for _, outcome := range outcomes {
		rank := "✗"
		switch {
		case outcome.Error != "":
			rank = "出错：" + outcome.Error
		case len(outcome.Case.Expect) == 0:
			rank = fmt.Sprintf("（无答案）塞进 %d 条", outcome.Injected)
		case outcome.Rank > 0:
			rank = fmt.Sprintf("%d", outcome.Rank)
		}
		query := outcome.Case.Query
		if len(outcome.Case.History) > 0 {
			query = strings.Join(outcome.Case.History, " / ") + " → " + query
		}
		fmt.Fprintf(&builder, "| %s | %s | %s | %s | %.1fs |\n", outcome.Case.Category, query, rank, strings.Join(outcome.Top, "、"), outcome.Elapsed.Seconds())
	}
	if err := os.MkdirAll("reports", 0o755); err != nil {
		return "", summary, err
	}
	path := filepath.Join("reports", "retrieval-"+time.Now().Format("20060102-150405")+".md")
	return path, summary, os.WriteFile(path, []byte(builder.String()), 0o644)
}
