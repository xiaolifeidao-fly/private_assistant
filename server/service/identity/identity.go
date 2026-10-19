// Package identity 管理两套账号：管理端（realm=manager）和用户端（realm=app）。
//
// 两套账号同表不同 realm，令牌也按 realm 隔离：app 的令牌进不了管理端接口，反之亦然。
// 令牌是随机串，库里只存 sha256；每次请求滑动续期，删行即吊销。
package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"common/keys"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	RealmManager = "manager"
	RealmApp     = "app"
)

var (
	ErrNotLogin        = errors.New("not login")
	ErrBadCredential   = errors.New("用户名或密码错误")
	ErrTooManyFailures = errors.New("登录失败次数过多，请 15 分钟后再试")
	ErrDisabled        = errors.New("账号已停用")
)

type Config struct {
	ManagerTTL      time.Duration // 管理端会话有效期
	AppTTL          time.Duration // 用户端会话有效期；手机主屏 PWA 要长一点，免得反复登录
	MaxLoginFail    int           // 窗口内同一用户名允许失败的次数
	LoginFailWindow time.Duration
}

type Service struct {
	db     *gorm.DB
	config Config
}

func New(database *gorm.DB, config Config) *Service {
	if config.ManagerTTL <= 0 {
		config.ManagerTTL = 7 * 24 * time.Hour
	}
	if config.AppTTL <= 0 {
		config.AppTTL = 90 * 24 * time.Hour
	}
	if config.MaxLoginFail <= 0 {
		config.MaxLoginFail = 10
	}
	if config.LoginFailWindow <= 0 {
		config.LoginFailWindow = 15 * time.Minute
	}
	return &Service{db: database, config: config}
}

// Account 是对外的账号视图，不含密码。
type Account struct {
	AccountKey  string     `json:"accountKey"`
	Realm       string     `json:"realm"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	Role        string     `json:"role"`
	FamilyKey   string     `json:"familyKey"`
	PersonKey   string     `json:"personKey"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type LoginInput struct {
	Realm     string
	Username  string
	Password  string
	ClientIP  string
	UserAgent string
}

type LoginResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	Account   Account   `json:"account"`
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" || input.Password == "" {
		return LoginResult{}, ErrBadCredential
	}
	var failures int64
	s.db.WithContext(ctx).Model(&loginRecordRow{}).
		Where("realm = ? AND username = ? AND success = 0 AND created_at > ?", input.Realm, username, time.Now().Add(-s.config.LoginFailWindow)).
		Count(&failures)
	if int(failures) >= s.config.MaxLoginFail {
		s.record(ctx, input, false, "locked")
		return LoginResult{}, ErrTooManyFailures
	}

	var row accountRow
	err := s.db.WithContext(ctx).Where("realm = ? AND username = ?", input.Realm, username).First(&row).Error
	if err != nil || bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(input.Password)) != nil {
		s.record(ctx, input, false, "bad_credential")
		return LoginResult{}, ErrBadCredential
	}
	if row.Status != "active" {
		s.record(ctx, input, false, "disabled")
		return LoginResult{}, ErrDisabled
	}

	token := keys.Random(32)
	now := time.Now()
	expires := now.Add(s.ttl(input.Realm))
	session := sessionRow{
		TokenHash: hashToken(token), AccountKey: row.AccountKey, Realm: row.Realm,
		ClientIP: input.ClientIP, UserAgent: truncate(input.UserAgent, 300),
		ExpiresAt: expires, CreatedAt: now, LastSeenAt: now,
	}
	if err := s.db.WithContext(ctx).Create(&session).Error; err != nil {
		return LoginResult{}, err
	}
	s.db.WithContext(ctx).Model(&accountRow{}).Where("id = ?", row.ID).Update("last_login_at", now)
	row.LastLoginAt = &now
	s.record(ctx, input, true, "")
	return LoginResult{Token: token, ExpiresAt: expires, Account: row.toDTO()}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).Where("token_hash = ?", hashToken(token)).Delete(&sessionRow{}).Error
}

// Authenticate 校验令牌并滑动续期。realm 由调用进程固定传入。
func (s *Service) Authenticate(ctx context.Context, realm, token string) (Account, error) {
	if token == "" {
		return Account{}, ErrNotLogin
	}
	var session sessionRow
	if err := s.db.WithContext(ctx).Where("token_hash = ? AND realm = ?", hashToken(token), realm).First(&session).Error; err != nil {
		return Account{}, ErrNotLogin
	}
	now := time.Now()
	if now.After(session.ExpiresAt) {
		s.db.WithContext(ctx).Delete(&session)
		return Account{}, ErrNotLogin
	}
	var row accountRow
	if err := s.db.WithContext(ctx).Where("account_key = ?", session.AccountKey).First(&row).Error; err != nil || row.Status != "active" {
		return Account{}, ErrNotLogin
	}
	// 续期写库按小时节流，避免每个请求都写一次。
	if now.Sub(session.LastSeenAt) > time.Hour {
		s.db.WithContext(ctx).Model(&sessionRow{}).Where("id = ?", session.ID).
			Updates(map[string]any{"last_seen_at": now, "expires_at": now.Add(s.ttl(realm))})
	}
	return row.toDTO(), nil
}

func (s *Service) ChangePassword(ctx context.Context, accountKey, oldPassword, newPassword string) error {
	var row accountRow
	if err := s.db.WithContext(ctx).Where("account_key = ?", accountKey).First(&row).Error; err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(oldPassword)) != nil {
		return errors.New("原密码不正确")
	}
	return s.setPassword(ctx, row, newPassword, true)
}

type AccountInput struct {
	Realm       string `json:"realm"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	FamilyKey   string `json:"familyKey"`
	PersonKey   string `json:"personKey"`
	Status      string `json:"status"`
}

func (s *Service) ListAccounts(ctx context.Context, realm string) ([]Account, error) {
	var rows []accountRow
	query := s.db.WithContext(ctx).Order("realm, id")
	if realm != "" {
		query = query.Where("realm = ?", realm)
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Account, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

func (s *Service) CreateAccount(ctx context.Context, input AccountInput) (Account, error) {
	if input.Realm != RealmManager && input.Realm != RealmApp {
		return Account{}, errors.New("realm 只能是 manager 或 app")
	}
	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" {
		return Account{}, errors.New("用户名不能为空")
	}
	if err := checkPassword(input.Password); err != nil {
		return Account{}, err
	}
	if err := checkRole(input.Realm, input.Role); err != nil {
		return Account{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return Account{}, err
	}
	now := time.Now()
	row := accountRow{
		AccountKey: keys.New("acc"), Realm: input.Realm, Username: input.Username,
		PasswordHash: string(hash), DisplayName: firstNonEmpty(input.DisplayName, input.Username),
		Role: firstNonEmpty(input.Role, "member"), FamilyKey: firstNonEmpty(input.FamilyKey, "default"),
		PersonKey: nullable(input.PersonKey), Status: "active", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return Account{}, errors.New("用户名已存在")
		}
		return Account{}, err
	}
	return row.toDTO(), nil
}

func (s *Service) UpdateAccount(ctx context.Context, accountKey string, input AccountInput) (Account, error) {
	var row accountRow
	if err := s.db.WithContext(ctx).Where("account_key = ?", accountKey).First(&row).Error; err != nil {
		return Account{}, err
	}
	updates := map[string]any{"updated_at": time.Now()}
	if input.DisplayName != "" {
		updates["display_name"] = input.DisplayName
	}
	if input.Role != "" {
		if err := checkRole(row.Realm, input.Role); err != nil {
			return Account{}, err
		}
		updates["role"] = input.Role
	}
	if input.Status != "" {
		updates["status"] = input.Status
	}
	updates["person_key"] = nullable(input.PersonKey)
	if err := s.db.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
		return Account{}, err
	}
	if input.Status != "" && input.Status != "active" {
		s.db.WithContext(ctx).Where("account_key = ?", accountKey).Delete(&sessionRow{})
	}
	if input.Password != "" {
		if err := s.setPassword(ctx, row, input.Password, true); err != nil {
			return Account{}, err
		}
	}
	s.db.WithContext(ctx).Where("account_key = ?", accountKey).First(&row)
	return row.toDTO(), nil
}

// EnsureAdmin 在 realm 下还没有任何账号时创建首个管理员。已有账号时什么都不做，
// 所以改了配置里的初始密码不会覆盖线上密码。
func (s *Service) EnsureAdmin(ctx context.Context, realm, username, password string) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&accountRow{}).Where("realm = ?", realm).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 || username == "" || password == "" {
		return false, nil
	}
	personKey := ""
	if realm == RealmApp {
		personKey = "me"
	}
	_, err := s.CreateAccount(ctx, AccountInput{Realm: realm, Username: username, Password: password, DisplayName: username, Role: "admin", PersonKey: personKey})
	return err == nil, err
}

func (s *Service) setPassword(ctx context.Context, row accountRow, password string, revokeSessions bool) error {
	if err := checkPassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&accountRow{}).Where("id = ?", row.ID).
		Updates(map[string]any{"password_hash": string(hash), "updated_at": time.Now()}).Error; err != nil {
		return err
	}
	if revokeSessions {
		return s.db.WithContext(ctx).Where("account_key = ?", row.AccountKey).Delete(&sessionRow{}).Error
	}
	return nil
}

func (s *Service) record(ctx context.Context, input LoginInput, success bool, reason string) {
	flag := 0
	if success {
		flag = 1
	}
	s.db.WithContext(ctx).Create(&loginRecordRow{
		Realm: input.Realm, Username: truncate(strings.TrimSpace(input.Username), 60), Success: flag,
		Reason: reason, ClientIP: input.ClientIP, CreatedAt: time.Now(),
	})
}

func (s *Service) ttl(realm string) time.Duration {
	if realm == RealmApp {
		return s.config.AppTTL
	}
	return s.config.ManagerTTL
}

func checkPassword(password string) error {
	if len(password) < 8 {
		return errors.New("密码至少 8 位")
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nullable(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	value = strings.TrimSpace(value)
	return &value
}
