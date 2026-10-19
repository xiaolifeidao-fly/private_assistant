package identity

import (
	"context"
	"errors"
	"sync"
	"time"
)

// App 账号的角色（pa_account.role）。家里每个人用的界面和能用的机器人不一样：
//   - admin / member：我自己，全部功能；
//   - spouse：老婆，只有万能通和提醒，不开我的私人助理和家人分身（见 01 · 8.1）；
//   - elder：奶奶（长辈模式），大字、语音优先，只有万事通、记一下、提醒三件事。
//
// 管理端账号只用 admin / member。
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleSpouse = "spouse"
	RoleElder  = "elder"
)

// 记一下在长辈模式里也交给万能通（它会用 remember 记下来）。
var roleBots = map[string][]string{
	RoleSpouse: {"recall", "remind"},
	RoleElder:  {"recall", "remind"},
}

// BotAllowed 判断这个角色能不能用某个机器人；没有单独限制的角色全部可用。
func BotAllowed(role, botKey string) bool {
	allowed, limited := roleBots[role]
	if !limited {
		return true
	}
	for _, key := range allowed {
		if key == botKey {
			return true
		}
	}
	return false
}

// checkRole：老婆、奶奶这两种角色只给手机端账号；空值按 member 处理。
func checkRole(realm, role string) error {
	switch role {
	case "", RoleAdmin, RoleMember:
		return nil
	case RoleSpouse, RoleElder:
		if realm == RealmApp {
			return nil
		}
	}
	return errors.New("角色不对：管理端只能是 admin / member，手机端还可以是 spouse / elder")
}

var systemCache struct {
	sync.Mutex
	accounts []string
	at       time.Time
}

// SystemAccounts 是家里「我自己」的手机端账号（admin 在前），后台任务以它们的身份看记录（common/viewer）。
// 一分钟缓存一次；查库失败时沿用上次的结果。
func (s *Service) SystemAccounts(ctx context.Context) []string {
	systemCache.Lock()
	defer systemCache.Unlock()
	if systemCache.accounts != nil && time.Since(systemCache.at) < time.Minute {
		return systemCache.accounts
	}
	var keys []string
	err := s.db.WithContext(ctx).Model(&accountRow{}).
		Where("realm = ? AND status = 'active' AND role IN ?", RealmApp, []string{RoleAdmin, RoleMember}).
		Order("role = 'admin' DESC, id").Pluck("account_key", &keys).Error
	if err != nil {
		return systemCache.accounts
	}
	if keys == nil {
		keys = []string{}
	}
	systemCache.accounts, systemCache.at = keys, time.Now()
	return keys
}

// PlanAccounts 是能改孩子饮食计划的手机端账号：我（admin / member）和老婆（spouse）。每周饮食对照推给他们。
func (s *Service) PlanAccounts(ctx context.Context) []string {
	var keys []string
	if err := s.db.WithContext(ctx).Model(&accountRow{}).
		Where("realm = ? AND status = 'active' AND role IN ?", RealmApp, []string{RoleAdmin, RoleMember, RoleSpouse}).
		Order("id").Pluck("account_key", &keys).Error; err != nil {
		return nil
	}
	return keys
}
