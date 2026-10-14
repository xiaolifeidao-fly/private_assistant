package db

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Migrate 按文件名顺序执行 fsys 里还没跑过的 *.sql。
//
// 两个 API 进程启动时都会调它，用 MySQL 的 GET_LOCK 串行化，谁先拿到锁谁执行，
// 另一个拿到锁时会发现已经执行过而跳过。每个文件内用 ";\n" 分句逐条执行，
// 执行完写一行 pa_schema_migration；中途失败不写记录，修好 SQL 后重启会重跑整个文件，
// 所以迁移文件里的语句要写成可重入的（CREATE TABLE IF NOT EXISTS / INSERT IGNORE）。
func Migrate(database *gorm.DB, fsys fs.FS) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, err := database.DB()
	if err != nil {
		return err
	}
	session, err := conn.Conn(ctx)
	if err != nil {
		return err
	}
	defer session.Close()

	var locked int
	if err := session.QueryRowContext(ctx, "SELECT GET_LOCK('pa_schema_migration', 60)").Scan(&locked); err != nil {
		return fmt.Errorf("获取迁移锁: %w", err)
	}
	if locked != 1 {
		return fmt.Errorf("60 秒内没拿到迁移锁")
	}
	defer session.ExecContext(context.Background(), "SELECT RELEASE_LOCK('pa_schema_migration')")

	if _, err := session.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS pa_schema_migration (
		name VARCHAR(200) NOT NULL PRIMARY KEY,
		applied_at DATETIME NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}

	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var count int
		if err := session.QueryRowContext(ctx, "SELECT COUNT(*) FROM pa_schema_migration WHERE name = ?", name).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		for _, statement := range splitStatements(string(content)) {
			if _, err := session.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("迁移 %s 失败: %w\n%s", name, err, statement)
			}
		}
		if _, err := session.ExecContext(ctx, "INSERT INTO pa_schema_migration (name, applied_at) VALUES (?, NOW())", name); err != nil {
			return err
		}
	}
	return nil
}

func splitStatements(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		lines = append(lines, line)
	}
	var statements []string
	for _, part := range strings.Split(strings.Join(lines, "\n"), ";\n") {
		if statement := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), ";")); statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}
