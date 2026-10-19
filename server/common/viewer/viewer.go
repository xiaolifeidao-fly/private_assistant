// Package viewer 管「谁在看」：记忆、原始记录、提醒每条都有归属账号（owner_account）和
// 可见范围（visibility），查询时只返回全家可见的，加上自己的。
//
//   - private：只有归属账号自己看得到（新记录默认）；
//   - family：全家都看得到。
//
// 隔离在数据访问层强制（gorm.go 的插件）：查这三张表的每条语句都自动加上可见范围条件，
// context 里没有声明身份的语句直接报错，不会默认放行。身份有三种，必须显式声明：
//
//   - With：登录账号（app-api 在登录校验后放进请求 context）；
//   - AsSystem：系统账号，即家里「我自己」的账号（role=admin/member，由 wire 注入）。
//     后台任务（人设卡、周洞察、派生提醒、对话学习）和管理端用它：看得到我的私有记录，看不到奶奶的；
//   - Unscoped：不过滤（领取待整理的记录、提醒调度、补算向量、评测这类系统动作）。
package viewer

import (
	"context"
	"errors"
	"strings"
)

const (
	Private = "private"
	Family  = "family"
)

// ErrNoIdentity 表示查受保护的表时 context 里没有声明身份。
var ErrNoIdentity = errors.New("没有声明是谁在查（viewer.With / AsSystem / Unscoped）")

type Viewer struct {
	Account string
	// Locked 为 true 时只能建私有记录，也不能改成全家可见（奶奶的账号）
	Locked bool
}

type mode int

const (
	modeUser mode = iota + 1
	modeSystem
	modeUnscoped
)

// scope 只用一个 context 键：后声明的身份覆盖先声明的（比如在 Unscoped 的后台任务里以某个账号的身份整理）。
type scope struct {
	mode   mode
	viewer Viewer
}

type key struct{}

// With 声明登录账号。账号为空等于没有声明。
func With(ctx context.Context, v Viewer) context.Context {
	if v.Account == "" {
		return ctx
	}
	return context.WithValue(ctx, key{}, scope{mode: modeUser, viewer: v})
}

// AsSystem 声明以系统账号（我自己）的身份看。也用来去掉登录账号：
// 派生提醒这类全家共用的同步，不能因为是谁触发的就只看到他的那份。
func AsSystem(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, scope{mode: modeSystem})
}

// Unscoped 声明不按可见范围过滤。
func Unscoped(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, scope{mode: modeUnscoped})
}

func current(ctx context.Context) (scope, bool) {
	if ctx == nil {
		return scope{}, false
	}
	value, ok := ctx.Value(key{}).(scope)
	return value, ok
}

// From 取出登录账号；系统、不过滤、没声明时都没有。
func From(ctx context.Context) (Viewer, bool) {
	value, ok := current(ctx)
	if !ok || value.mode != modeUser {
		return Viewer{}, false
	}
	return value.viewer, true
}

// SystemAccounts 返回系统账号（家里「我自己」的手机端账号，管理员在前）。由 wire 注入。
var SystemAccounts = func(ctx context.Context) []string { return nil }

// Accounts 是能看到其私有记录的账号；unscoped 为 true 表示不过滤；没声明身份返回 ErrNoIdentity。
func Accounts(ctx context.Context) (accounts []string, unscoped bool, err error) {
	value, ok := current(ctx)
	if !ok {
		return nil, false, ErrNoIdentity
	}
	switch value.mode {
	case modeUnscoped:
		return nil, true, nil
	case modeUser:
		return []string{value.viewer.Account}, false, nil
	default:
		return SystemAccounts(ctx), false, nil
	}
}

// Clause 返回加在手写 SQL（Raw、Joins）里的条件；prefix 是表别名（如 "m."），单表传空串。
// 没声明身份时返回永假条件，宁可查不到也不多给。
func Clause(ctx context.Context, prefix string) (string, []any) {
	accounts, unscoped, err := Accounts(ctx)
	switch {
	case err != nil:
		return "1 = 0", nil
	case unscoped:
		return "1 = 1", nil
	case len(accounts) == 0:
		return prefix + "visibility = 'family'", nil
	}
	return "(" + prefix + "visibility = 'family' OR " + prefix + "owner_account IN ?)", []any{accounts}
}

// CanSee 判断一条记录对当前 context 是否可见。
func CanSee(ctx context.Context, owner, visibility string) bool {
	accounts, unscoped, err := Accounts(ctx)
	if err != nil {
		return false
	}
	if unscoped || visibility == Family {
		return true
	}
	for _, account := range accounts {
		if account != "" && account == owner {
			return true
		}
	}
	return false
}

// IsSystem 判断当前是不是「我自己」在看：系统身份、不过滤，或者登录的是系统账号。
// 人设卡、关系要点这些由我的私有记录整理出来的东西，只给这种情况看。没声明身份时为否。
func IsSystem(ctx context.Context) bool {
	value, ok := current(ctx)
	if !ok {
		return false
	}
	if value.mode != modeUser {
		return true
	}
	for _, account := range SystemAccounts(ctx) {
		if account == value.viewer.Account {
			return true
		}
	}
	return false
}

// Owner 是新记录的归属账号：登录账号；系统身份下取第一个系统账号；没声明身份为空。
func Owner(ctx context.Context) string {
	value, ok := current(ctx)
	if !ok {
		return ""
	}
	if value.mode == modeUser {
		return value.viewer.Account
	}
	if accounts := SystemAccounts(ctx); len(accounts) > 0 {
		return accounts[0]
	}
	return ""
}

// Resolve 把请求里的可见范围规范成 private / family：没说就私有；被锁定的账号只能私有；
// 没有归属账号（还没建手机端账号、评测库）时只能是全家可见，否则谁也看不到。
func Resolve(ctx context.Context, requested string) string {
	if v, ok := From(ctx); ok && v.Locked {
		return Private
	}
	if Owner(ctx) == "" {
		return Family
	}
	if strings.TrimSpace(requested) == Family {
		return Family
	}
	return Private
}

// CanChange 判断当前身份能不能改这条记录的可见范围：只有归属人自己（被锁定的账号不能改）；
// 系统身份可以改系统账号名下的。
func CanChange(ctx context.Context, owner string) bool {
	value, ok := current(ctx)
	if !ok || owner == "" {
		return false
	}
	if value.mode == modeUser {
		return !value.viewer.Locked && value.viewer.Account == owner
	}
	for _, account := range SystemAccounts(ctx) {
		if account == owner {
			return true
		}
	}
	return false
}
