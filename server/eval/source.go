package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 从线上库复制的配置表。只读线上库；评测库里先清空再整表写入。
// 记忆和对话不复制 —— 评测始终用 fixture 里的虚构数据，真实家庭数据不进评测库。
var sourceTables = []string{"pa_bot", "pa_skill", "pa_setting"}

// 检索评测要和线上用同一个向量模型（和改写检索词的模型），所以复制模型配置（含 Key，同一台库，只在评测库里用）。
var modelTables = []string{"pa_llm_provider", "pa_llm_route"}

func copyFromSource(ctx context.Context, target *gorm.DB, dsn string) error {
	return copyTables(ctx, target, dsn, sourceTables)
}

func copyTables(ctx context.Context, target *gorm.DB, dsn string, tables []string) error {
	if dsn == "" {
		return errors.New("没有配置 source.sqlconn（线上库连接，建议用只读账号）")
	}
	source, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	if err != nil {
		return err
	}
	if pool, err := source.DB(); err == nil {
		defer pool.Close()
	}
	var sourceName, targetName string
	source.Raw("SELECT DATABASE()").Scan(&sourceName)
	target.Raw("SELECT DATABASE()").Scan(&targetName)
	if strings.EqualFold(sourceName, targetName) {
		return fmt.Errorf("线上库和评测库是同一个库（%s）", sourceName)
	}
	for _, table := range tables {
		var rows []map[string]any
		if err := source.WithContext(ctx).Table(table).Find(&rows).Error; err != nil {
			return fmt.Errorf("读 %s: %w", table, err)
		}
		if err := target.WithContext(ctx).Exec("DELETE FROM " + table).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := target.WithContext(ctx).Table(table).Create(&rows).Error; err != nil {
				return fmt.Errorf("写 %s: %w", table, err)
			}
		}
		log.Printf("从 %s 复制了 %s %d 行", sourceName, table, len(rows))
	}
	return nil
}
