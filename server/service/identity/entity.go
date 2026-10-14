package identity

import "time"

// 实体类型不导出：别的领域包拿不到它们，也就写不了这几张表。

type accountRow struct {
	ID           int64
	AccountKey   string
	Realm        string
	Username     string
	PasswordHash string
	DisplayName  string
	Role         string
	FamilyKey    string
	PersonKey    *string
	Status       string
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (accountRow) TableName() string { return "pa_account" }

func (row accountRow) toDTO() Account {
	account := Account{
		AccountKey: row.AccountKey, Realm: row.Realm, Username: row.Username, DisplayName: row.DisplayName,
		Role: row.Role, FamilyKey: row.FamilyKey, Status: row.Status, LastLoginAt: row.LastLoginAt, CreatedAt: row.CreatedAt,
	}
	if row.PersonKey != nil {
		account.PersonKey = *row.PersonKey
	}
	return account
}

type sessionRow struct {
	ID         int64
	TokenHash  string
	AccountKey string
	Realm      string
	ClientIP   string
	UserAgent  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
}

func (sessionRow) TableName() string { return "pa_session" }

type loginRecordRow struct {
	ID        int64
	Realm     string
	Username  string
	Success   int
	Reason    string
	ClientIP  string
	CreatedAt time.Time
}

func (loginRecordRow) TableName() string { return "pa_login_record" }
