package db

import (
	"sync"

	"gorm.io/gorm"
)

var (
	database *gorm.DB
	mu       sync.RWMutex
)

// SetDatabase 由 httpx.Boot 在进程启动时调用一次。
func SetDatabase(value *gorm.DB) {
	mu.Lock()
	defer mu.Unlock()
	database = value
}

// Database 返回进程共享的连接池。
func Database() *gorm.DB {
	mu.RLock()
	defer mu.RUnlock()
	return database
}
