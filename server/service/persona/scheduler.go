package persona

import (
	"context"
	"log"
	"time"
)

// 每 10 分钟看一次：只有记忆变了、又稳定下来的人才会去判断，检查本身很便宜。
// 判断也只看新记忆；确实改变了她的人设才整卡重写，其余的留在记忆里供对话检索。
const refreshInterval = 10 * time.Minute

// RunScheduler 定时检查并刷新人设卡，直到 ctx 结束。
// 两个进程都会跑，用 MySQL 的 GET_LOCK 保证同一时刻只有一个在刷新。
func (s *Service) RunScheduler(ctx context.Context) {
	timer := time.NewTimer(2 * time.Minute) // 启动后稍等，别和迁移、补算向量挤在一起
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.refreshAll(ctx)
		timer.Reset(refreshInterval)
	}
}

func (s *Service) refreshAll(ctx context.Context) {
	sqlDB, err := s.db.DB()
	if err != nil {
		return
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		log.Printf("人设卡刷新: %v", err)
		return
	}
	defer conn.Close()
	var locked int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK('pa_persona_refresh', 0)").Scan(&locked); err != nil || locked != 1 {
		return // 另一个进程正在刷新
	}
	defer conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK('pa_persona_refresh')")

	var families []string
	if err := s.db.WithContext(ctx).Table("pa_family").Pluck("family_key", &families).Error; err != nil {
		log.Printf("人设卡刷新: %v", err)
		return
	}
	for _, familyKey := range families {
		jobCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		s.RefreshDue(jobCtx, familyKey)
		cancel()
	}
}
