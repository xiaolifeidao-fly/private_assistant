package viewer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testMemory struct {
	ID           int64
	MemoryKey    string
	FamilyKey    string
	Title        string
	OwnerAccount string
	Visibility   string
}

func (testMemory) TableName() string { return "pa_memory" }

type testNote struct {
	NoteKey string
	RawText string
}

type testLink struct {
	ID        int64
	MemoryKey string
}

func (testLink) TableName() string { return "pa_memory_link" }

// dryRun 只生成 SQL 不连库：检查插件加的条件和拒绝。
func dryRun(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(mysql.New(mysql.Config{DSN: "u:p@tcp(127.0.0.1:1)/x", SkipInitializeWithVersion: true}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Use(Plugin{}); err != nil {
		t.Fatal(err)
	}
	return database
}

const visibleSQL = "(`pa_memory`.`visibility` = ? OR `pa_memory`.`owner_account` IN (?))"

func TestQueryAddsVisibility(t *testing.T) {
	withSystem(t, "acc_me")
	database := dryRun(t)
	grandma := With(context.Background(), Viewer{Account: "acc_nai", Locked: true})

	var rows []testMemory
	result := database.WithContext(grandma).Where("family_key = ?", "default").Find(&rows)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	sql := result.Statement.SQL.String()
	if !strings.Contains(sql, visibleSQL) || !strings.Contains(sql, "family_key = ?") {
		t.Fatalf("查询没有加上可见范围：%s", sql)
	}
	if vars := result.Statement.Vars; vars[len(vars)-1] != "acc_nai" {
		t.Fatalf("应按奶奶的账号过滤，参数 %v", vars)
	}

	// 计数、单条、Pluck 都一样
	var count int64
	result = database.WithContext(grandma).Model(&testMemory{}).Count(&count)
	if !strings.Contains(result.Statement.SQL.String(), visibleSQL) {
		t.Fatalf("计数没有加上可见范围：%s", result.Statement.SQL.String())
	}
	var keys []string
	result = database.WithContext(grandma).Model(&testMemory{}).Pluck("memory_key", &keys)
	if !strings.Contains(result.Statement.SQL.String(), visibleSQL) {
		t.Fatalf("Pluck 没有加上可见范围：%s", result.Statement.SQL.String())
	}

	// 用 Table 指定表名、结果是别的结构体时也加
	var notes []testNote
	result = database.WithContext(grandma).Table("pa_source_note").Find(&notes)
	if !strings.Contains(result.Statement.SQL.String(), "`pa_source_note`.`owner_account` IN (?)") {
		t.Fatalf("Table 查询没有加上可见范围：%s", result.Statement.SQL.String())
	}
}

func TestSystemAndUnscoped(t *testing.T) {
	withSystem(t, "acc_me")
	database := dryRun(t)

	var rows []testMemory
	result := database.WithContext(AsSystem(context.Background())).Find(&rows)
	if vars := result.Statement.Vars; !strings.Contains(result.Statement.SQL.String(), visibleSQL) || vars[len(vars)-1] != "acc_me" {
		t.Fatalf("系统身份应按我自己的账号过滤：%s %v", result.Statement.SQL.String(), result.Statement.Vars)
	}
	result = database.WithContext(Unscoped(context.Background())).Find(&rows)
	if strings.Contains(result.Statement.SQL.String(), "owner_account") {
		t.Fatalf("Unscoped 不应过滤：%s", result.Statement.SQL.String())
	}
	// 后声明的覆盖先声明的：在 Unscoped 的后台任务里以奶奶的身份整理
	inner := With(Unscoped(context.Background()), Viewer{Account: "acc_nai"})
	result = database.WithContext(inner).Find(&rows)
	if !strings.Contains(result.Statement.SQL.String(), visibleSQL) {
		t.Fatalf("With 应覆盖 Unscoped：%s", result.Statement.SQL.String())
	}
}

func TestNoIdentityIsRejected(t *testing.T) {
	database := dryRun(t)
	var rows []testMemory
	if err := database.WithContext(context.Background()).Find(&rows).Error; !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("没声明身份应拒绝查询，得到 %v", err)
	}
	if err := database.Model(&testMemory{}).Where("id = 1").Update("title", "x").Error; !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("没声明身份应拒绝更新，得到 %v", err)
	}
	if err := database.Where("id = 1").Delete(&testMemory{}).Error; !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("没声明身份应拒绝删除，得到 %v", err)
	}
	if err := database.Create(&testMemory{OwnerAccount: "a", Visibility: Family}).Error; !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("没声明身份应拒绝新建，得到 %v", err)
	}
	// 不受保护的表不管
	var links []testLink
	if err := database.Find(&links).Error; err != nil {
		t.Fatalf("不受保护的表不应拦：%v", err)
	}
}

func TestUpdateDeleteScoped(t *testing.T) {
	withSystem(t, "acc_me")
	database := dryRun(t)
	ctx := With(context.Background(), Viewer{Account: "acc_wife"})
	result := database.WithContext(ctx).Model(&testMemory{}).Where("memory_key = ?", "mem_1").Update("title", "x")
	if !strings.Contains(result.Statement.SQL.String(), visibleSQL) {
		t.Fatalf("更新没有加上可见范围：%s", result.Statement.SQL.String())
	}
	result = database.WithContext(ctx).Where("memory_key = ?", "mem_1").Delete(&testMemory{})
	if !strings.Contains(result.Statement.SQL.String(), visibleSQL) {
		t.Fatalf("删除没有加上可见范围：%s", result.Statement.SQL.String())
	}
}

func TestCreateOnlyVisibleRows(t *testing.T) {
	withSystem(t, "acc_me")
	database := dryRun(t)
	grandma := With(context.Background(), Viewer{Account: "acc_nai", Locked: true})

	if err := database.WithContext(grandma).Create(&testMemory{OwnerAccount: "acc_nai", Visibility: Private}).Error; err != nil {
		t.Fatalf("写自己的私有记录应允许：%v", err)
	}
	if err := database.WithContext(grandma).Create(&testMemory{OwnerAccount: "acc_me", Visibility: Private}).Error; err == nil {
		t.Fatal("不能替别人写私有记录")
	}
	if err := database.WithContext(grandma).Create(&[]testMemory{{OwnerAccount: "acc_nai", Visibility: Private}, {OwnerAccount: "", Visibility: Private}}).Error; err == nil {
		t.Fatal("批量写里有一条看不到的也要拒绝")
	}
	if err := database.WithContext(Unscoped(context.Background())).Create(&testMemory{OwnerAccount: "acc_me", Visibility: Private}).Error; err != nil {
		t.Fatalf("Unscoped 可以写：%v", err)
	}
}

func TestRawAndJoins(t *testing.T) {
	withSystem(t, "acc_me")
	database := dryRun(t)
	ctx := With(context.Background(), Viewer{Account: "acc_nai"})

	if err := database.WithContext(ctx).Exec("UPDATE pa_memory SET title = ? WHERE id = 1", "x").Error; err == nil {
		t.Fatal("手写 SQL 不带可见范围条件应拒绝")
	}
	clause, args := Clause(ctx, "")
	if err := database.WithContext(ctx).Exec("UPDATE pa_memory SET title = ? WHERE id = 1 AND "+clause, append([]any{"x"}, args...)...).Error; err != nil {
		t.Fatalf("带了条件的手写 SQL 应允许：%v", err)
	}
	if err := database.WithContext(context.Background()).Exec("DELETE FROM pa_memory_link WHERE id = 1").Error; err != nil {
		t.Fatalf("不受保护的表不应拦：%v", err)
	}

	var keys []string
	err := database.WithContext(ctx).Model(&testLink{}).Joins("JOIN pa_memory m ON m.memory_key = pa_memory_link.memory_key").
		Pluck("pa_memory_link.memory_key", &keys).Error
	if err == nil {
		t.Fatal("连到受保护的表不带条件应拒绝")
	}
	joinClause, joinArgs := Clause(ctx, "m.")
	err = database.WithContext(ctx).Model(&testLink{}).Joins("JOIN pa_memory m ON m.memory_key = pa_memory_link.memory_key").
		Where(joinClause, joinArgs...).Pluck("pa_memory_link.memory_key", &keys).Error
	if err != nil {
		t.Fatalf("带了条件的连表应允许：%v", err)
	}
}
