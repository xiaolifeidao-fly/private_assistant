// eval 是回答质量的评测集（设计文档 05 · 3）。
//
// 在单独的评测库里装一份虚构的家庭记忆（fixture.json），按 cases/*.json 逐条真实地跑对话，
// 检查工具和手册有没有用对、引用有没有出处、有没有说不该说的话，再让模型按 rubric 打分。
//
//	cd server/eval && go run . [-case 婆媳] [-repeat 3] [-skip-judge] [-from-source]
//	cd server/eval && go run . -retrieval [-label 说明]    只测检索命中率，见 retrieval.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"common/middleware/db"
	"common/middleware/httpx"
	"migrations"
	"service/wire"
)

func main() {
	caseFilter := flag.String("case", "", "只跑名字包含它的用例")
	repeat := flag.Int("repeat", 1, "每个用例跑几遍（看稳定性）")
	skipJudge := flag.Bool("skip-judge", false, "不调用模型打分，只跑程序断言")
	fromSource := flag.Bool("from-source", false, "从线上库（source.sqlconn）只读地复制机器人、手册、对话规则，评测管理端里改过的版本")
	retrievalOnly := flag.Bool("retrieval", false, "只做检索评测：看每轮自动检索把该找的记忆排在第几（retrieval/cases.json）")
	label := flag.String("label", "", "检索评测报告的标题说明，如「改写检索词之后」")
	flag.Parse()

	database := httpx.Boot("eval")
	var name string
	if err := database.Raw("SELECT DATABASE()").Scan(&name).Error; err != nil {
		log.Fatalf("查询库名失败: %v", err)
	}
	// 每次评测都会清空记忆和对话，绝不能连到真实数据的库
	if !strings.Contains(strings.ToLower(name), "eval") {
		log.Fatalf("评测库名必须包含 eval，当前是 %q。请建一个单独的库，例如 private_assistant_eval", name)
	}
	if err := db.Migrate(database, migrations.FS()); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	options := wire.Options{
		AnthropicAPIKey:  httpx.Property("llm.anthropic.api_key"),
		AnthropicBaseURL: httpx.Property("llm.anthropic.base_url"),
		EmbeddingBaseURL: httpx.Property("llm.embedding.base_url"),
		EmbeddingAPIKey:  httpx.Property("llm.embedding.api_key"),
		StorageDir:       filepath.Join(os.TempDir(), "pa-eval-data"),
		VectorStore:      "mysql", // 评测库每次清空重建，向量跟着库走
	}
	ctx := context.Background()
	if *retrievalOnly {
		// 用线上同一套模型配置（向量模型、改写检索词的模型），命中率才有参考价值
		if dsn := httpx.Property("source.sqlconn"); dsn != "" {
			if err := copyTables(ctx, database, dsn, modelTables); err != nil {
				log.Fatalf("从线上库复制模型配置失败: %v", err)
			}
		}
	}
	services := wire.Build(database, options)

	refs, err := resetAndSeed(ctx, database, services, options)
	if err != nil {
		log.Fatalf("装评测数据失败: %v", err)
	}
	if *retrievalOnly {
		outcomes, err := runRetrieval(ctx, services, refs, *caseFilter)
		if err != nil {
			log.Fatalf("检索评测失败: %v", err)
		}
		path, summary, err := writeRetrievalReport(outcomes, *label)
		if err != nil {
			log.Printf("写报告失败: %v", err)
		}
		log.Printf("| 类别 | 用例 | 第 1 名 | 前 3 | 进上下文 | MRR | 无答案塞进 | 平均塞进 |")
		log.Printf("%s", summary)
		log.Printf("报告：%s", path)
		return
	}
	configLabel := "代码里的出厂内容"
	if *fromSource {
		if err := copyFromSource(ctx, database, httpx.Property("source.sqlconn")); err != nil {
			log.Fatalf("从线上库复制配置失败: %v", err)
		}
		configLabel = "线上库里的机器人、手册、对话规则"
	}
	log.Printf("评测的配置：%s", configLabel)
	cases, err := loadCases("cases", *caseFilter)
	if err != nil {
		log.Fatalf("读取用例失败: %v", err)
	}
	if len(cases) == 0 {
		log.Fatalf("没有匹配的用例")
	}

	runner := &runner{services: services, refs: refs, judge: !*skipJudge}
	var results []result
	for _, item := range cases {
		for round := 1; round <= *repeat; round++ {
			label := item.Name
			if *repeat > 1 {
				label = fmt.Sprintf("%s #%d", item.Name, round)
			}
			log.Printf("▶ %s", label)
			outcome := runner.run(ctx, item)
			outcome.Label = label
			for _, failure := range outcome.Failures {
				log.Printf("  ✗ %s", failure)
			}
			if outcome.passed() {
				log.Printf("  ✓ 通过")
			}
			results = append(results, outcome)
		}
	}

	path, err := writeReport(results, time.Now(), configLabel)
	if err != nil {
		log.Printf("写报告失败: %v", err)
	}
	passed := 0
	for _, outcome := range results {
		if outcome.passed() {
			passed++
		}
	}
	log.Printf("通过 %d / %d，报告：%s", passed, len(results), path)
	if passed < len(results) {
		os.Exit(1)
	}
}
