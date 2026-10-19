package viewer

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 受保护的表：每行都有 owner_account / visibility。
var protectedTables = map[string]bool{"pa_memory": true, "pa_source_note": true, "pa_reminder": true}

// 手写 SQL 里提到受保护的表（pa_memory_link、pa_memory_embedding 这类不算）。
var tableMention = regexp.MustCompile(`\bpa_(memory|source_note|reminder)\b`)

// Plugin 在数据访问层强制可见范围，业务代码写漏了条件也查不到别人的私有记录：
//
//   - 查询、计数、更新、删除受保护的表：自动加上 (visibility='family' OR owner_account IN 当前账号)；
//   - 新建：只能建当前身份看得到的行，不能替别人写私有记录；
//   - Joins 连到受保护的表、手写 SQL 查受保护的表：必须自带可见范围条件（用 Clause 拼）；
//   - context 里没有声明身份（With / AsSystem / Unscoped）的，以上全部直接报错。
//
// 用法：database.Use(viewer.Plugin{})。
type Plugin struct{}

func (Plugin) Name() string { return "viewer" }

func (Plugin) Initialize(db *gorm.DB) error {
	callbacks := db.Callback()
	steps := []error{
		callbacks.Query().Before("gorm:query").Register("viewer:query", filter),
		callbacks.Row().Before("gorm:row").Register("viewer:row", filter),
		callbacks.Update().Before("gorm:update").Register("viewer:update", filter),
		callbacks.Delete().Before("gorm:delete").Register("viewer:delete", filter),
		callbacks.Create().Before("gorm:create").Register("viewer:create", checkCreate),
		callbacks.Raw().Before("gorm:raw").Register("viewer:raw", checkRaw),
	}
	for _, err := range steps {
		if err != nil {
			return err
		}
	}
	return nil
}

func filter(db *gorm.DB) {
	if db.Error != nil {
		return
	}
	stmt := db.Statement
	if stmt.SQL.Len() > 0 { // Raw(...).Scan / Rows
		checkRaw(db)
		return
	}
	if protectedTables[stmt.Table] {
		accounts, unscoped, err := Accounts(stmt.Context)
		if err != nil {
			db.AddError(fmt.Errorf("查 %s：%w", stmt.Table, err))
			return
		}
		if !unscoped {
			stmt.AddClause(clause.Where{Exprs: []clause.Expression{visibleExpr(accounts)}})
		}
	}
	checkJoins(db)
}

// visibleExpr：(visibility = 'family' OR owner_account IN ?)，列名带上当前表名，连表时不会有歧义。
func visibleExpr(accounts []string) clause.Expression {
	visibility := clause.Column{Table: clause.CurrentTable, Name: "visibility"}
	if len(accounts) == 0 {
		return clause.Expr{SQL: "? = ?", Vars: []any{visibility, Family}}
	}
	owner := clause.Column{Table: clause.CurrentTable, Name: "owner_account"}
	return clause.Expr{SQL: "(? = ? OR ? IN ?)", Vars: []any{visibility, Family, owner, accounts}}
}

// checkJoins：连到受保护的表时插件没法替别名加条件，要求调用方自己用 Clause 加。
func checkJoins(db *gorm.DB) {
	stmt := db.Statement
	for _, join := range stmt.Joins {
		if !tableMention.MatchString(join.Name) {
			continue
		}
		_, unscoped, err := Accounts(stmt.Context)
		if err != nil {
			db.AddError(fmt.Errorf("连表查 %s：%w", join.Name, err))
			return
		}
		if !unscoped && !whereHasVisibility(stmt) {
			db.AddError(fmt.Errorf("连表查受保护的表必须用 viewer.Clause 加可见范围条件：%s", join.Name))
		}
		return
	}
}

func whereHasVisibility(stmt *gorm.Statement) bool {
	where, ok := stmt.Clauses["WHERE"].Expression.(clause.Where)
	if !ok {
		return false
	}
	for _, expression := range where.Exprs {
		if expr, ok := expression.(clause.Expr); ok && strings.Contains(expr.SQL, "visibility") {
			return true
		}
	}
	return false
}

// checkRaw：手写 SQL 改写不了，只检查——提到受保护的表就必须自带可见范围条件（Clause 拼出来的都含 visibility）。
func checkRaw(db *gorm.DB) {
	if db.Error != nil {
		return
	}
	sql := db.Statement.SQL.String()
	if !tableMention.MatchString(sql) {
		return
	}
	_, unscoped, err := Accounts(db.Statement.Context)
	if err != nil {
		db.AddError(fmt.Errorf("手写 SQL 查受保护的表：%w", err))
		return
	}
	if !unscoped && !strings.Contains(sql, "visibility") {
		db.AddError(fmt.Errorf("手写 SQL 查受保护的表必须用 viewer.Clause 加可见范围条件"))
	}
}

// checkCreate：只能写当前身份看得到的行。
func checkCreate(db *gorm.DB) {
	if db.Error != nil {
		return
	}
	stmt := db.Statement
	if !protectedTables[stmt.Table] {
		return
	}
	_, unscoped, err := Accounts(stmt.Context)
	if err != nil {
		db.AddError(fmt.Errorf("写 %s：%w", stmt.Table, err))
		return
	}
	if unscoped {
		return
	}
	if stmt.Schema == nil {
		db.AddError(fmt.Errorf("写 %s 必须用带 owner_account / visibility 字段的结构体", stmt.Table))
		return
	}
	ownerField, visibilityField := stmt.Schema.LookUpField("owner_account"), stmt.Schema.LookUpField("visibility")
	if ownerField == nil || visibilityField == nil {
		db.AddError(fmt.Errorf("写 %s 必须带 owner_account / visibility 字段", stmt.Table))
		return
	}
	check := func(row reflect.Value) {
		owner, _ := ownerField.ValueOf(stmt.Context, row)
		visibility, _ := visibilityField.ValueOf(stmt.Context, row)
		ownerText, _ := owner.(string)
		visibilityText, _ := visibility.(string)
		if !CanSee(stmt.Context, ownerText, visibilityText) {
			db.AddError(fmt.Errorf("不能写别人的私有记录（%s owner=%q visibility=%q）", stmt.Table, ownerText, visibilityText))
		}
	}
	value := reflect.Indirect(stmt.ReflectValue)
	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			check(reflect.Indirect(value.Index(i)))
		}
	case reflect.Struct:
		check(value)
	}
}
