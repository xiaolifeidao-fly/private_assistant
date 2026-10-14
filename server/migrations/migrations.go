// Package migrations 把 sql/*.sql 嵌进二进制，由 common/middleware/db.Migrate 执行。
//
// 规则：已经发布的文件不改，改表结构一律追加新文件（0002_xxx.sql …）。
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed sql/*.sql
var files embed.FS

// FS 返回迁移文件目录。
func FS() fs.FS {
	sub, err := fs.Sub(files, "sql")
	if err != nil {
		panic(err)
	}
	return sub
}
