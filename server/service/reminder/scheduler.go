package reminder

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

// RunScheduler 每分钟派生和投递提醒。MySQL 命名锁保证两个 API 进程不重复工作。
func (s *Service) RunScheduler(ctx context.Context) {
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.runOnce(ctx)
			timer.Reset(time.Minute)
		}
	}
}

func (s *Service) runOnce(ctx context.Context) {
	sqlDB, err := s.db.DB()
	if err != nil {
		return
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return
	}
	defer conn.Close()
	var locked int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK('pa_reminder_worker', 0)").Scan(&locked); err != nil || locked != 1 {
		return
	}
	defer conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK('pa_reminder_worker')")
	// 进程可能在抢占后、投递前崩溃；超过 10 分钟的 sending 重新进重试队列。
	s.db.WithContext(ctx).Model(&reminderRow{}).
		Where("status = ? AND updated_at < ?", StatusSending, time.Now().Add(-10*time.Minute)).
		Updates(map[string]any{"status": StatusFailed, "attempts": gorm.Expr("attempts + 1"), "last_error": "上次投递中断，已自动重试", "updated_at": time.Now()})

	var families []string
	if err := s.db.WithContext(ctx).Table("pa_family").Pluck("family_key", &families).Error; err != nil {
		return
	}
	for _, familyKey := range families {
		if err := s.SyncDerived(ctx, familyKey, time.Now()); err != nil {
			log.Printf("派生提醒 %s: %v", familyKey, err)
		}
		if s.PushEnabled() {
			s.sendDue(ctx, familyKey)
		}
	}
}

func (s *Service) sendDue(ctx context.Context, familyKey string) {
	today := atClock(time.Now(), 0)
	// 日报和节日/生日/行程通知过了事件日就不再补发，避免新设备刚订阅时收到一串旧通知。
	s.db.WithContext(ctx).Model(&reminderRow{}).
		Where("family_key = ? AND status IN ? AND ((kind = 'daily' AND remind_at < ?) OR (kind IN ? AND event_at < ?))",
			familyKey, []string{StatusScheduled, StatusFailed}, today, []string{"birthday", "festival", "trip"}, today).
		Updates(map[string]any{"status": StatusCancelled, "updated_at": time.Now()})
	for i := 0; i < 30; i++ {
		var row reminderRow
		err := s.db.WithContext(ctx).Where("family_key = ? AND status IN ? AND remind_at <= ? AND attempts < 5",
			familyKey, []string{StatusScheduled, StatusFailed}, time.Now()).Order("remind_at, id").First(&row).Error
		if err != nil {
			return
		}
		claim := s.db.WithContext(ctx).Model(&reminderRow{}).Where("id = ? AND status IN ?", row.ID, []string{StatusScheduled, StatusFailed}).
			Updates(map[string]any{"status": StatusSending, "updated_at": time.Now()})
		if claim.Error != nil || claim.RowsAffected != 1 {
			continue
		}
		sent, _, sendErr := s.deliver(ctx, familyKey, row.toDTO())
		if sent > 0 {
			now := time.Now()
			s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"status": StatusSent, "sent_at": now, "last_error": "", "updated_at": now})
			continue
		}
		// 还没有订阅时保留 scheduled，用户之后开启推送仍能收到未过时的提醒。
		if sendErr == nil {
			s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"status": StatusScheduled, "updated_at": time.Now()})
			return
		}
		s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"status": StatusFailed, "attempts": row.Attempts + 1,
			"last_error": truncate(sendErr.Error(), 1000), "updated_at": time.Now()})
	}
}
