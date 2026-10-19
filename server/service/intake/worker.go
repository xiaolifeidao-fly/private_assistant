package intake

import (
	"context"
	"fmt"
	"log"
	"time"

	"common/middleware/db"
	"common/viewer"
)

// 原始记录表本身就是队列：worker 按 extract_status 抢占一条处理。
// 两个 API 进程都可以跑 worker，抢占用条件更新保证一条记录只被一个 worker 拿到；
// 进程中途退出时 running 状态超过 staleAfter 会被重新领取。
const (
	pollInterval = 15 * time.Second
	staleAfter   = 10 * time.Minute
	retryBackoff = 2 * time.Minute
	jobTimeout   = 5 * time.Minute
)

// Wake 让本进程的 worker 立刻检查队列（新记录入队时调用）。
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run 阻塞运行 worker，直到 ctx 结束。
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		for {
			worked, err := s.runOnce(ctx)
			if err != nil {
				log.Printf("抽取 worker: %v", err)
			}
			if !worked || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

// runOnce 领取并处理一条记录；队列为空时返回 false。
func (s *Service) runOnce(ctx context.Context) (bool, error) {
	row, ok, err := s.claim(ctx)
	if err != nil || !ok {
		return false, err
	}
	return true, s.finish(ctx, row)
}

// finish 处理一条已经抢到的记录并写回状态。
func (s *Service) finish(ctx context.Context, row noteRow) error {
	jobCtx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	result, processErr := s.safeProcess(jobCtx, row)
	now := time.Now()
	updates := map[string]any{"extract_result": db.MustJSON(result)}
	if processErr == nil {
		// 原文也切段算向量，整理漏掉的细节以后还能搜到（失败不影响，启动时会补）
		go func(familyKey, noteKey string) {
			ctx, cancel := context.WithTimeout(viewer.Unscoped(context.Background()), 2*time.Minute)
			defer cancel()
			if err := s.memory.IndexNote(ctx, familyKey, noteKey); err != nil {
				log.Printf("原始记录 %s 原文向量化失败（稍后补算）: %v", noteKey, err)
			}
		}(row.FamilyKey, row.NoteKey)
		updates["extract_status"] = StatusDone
		updates["extracted_at"] = now
		updates["extract_error"] = nil
	} else {
		message := processErr.Error()
		if len([]rune(message)) > 900 {
			message = string([]rune(message)[:900])
		}
		updates["extract_error"] = message
		updates["claimed_at"] = now // 退避起点
		if row.ExtractAttempts+1 >= maxAttempts {
			updates["extract_status"] = StatusFailed
		} else {
			updates["extract_status"] = StatusQueued
		}
		log.Printf("抽取 %s 失败（第 %d 次）: %v", row.NoteKey, row.ExtractAttempts+1, processErr)
	}
	return s.db.WithContext(context.WithoutCancel(ctx)).Model(&noteRow{}).Where("id = ?", row.ID).Updates(updates).Error
}

// ProcessNow 当场整理一条刚提交的记录并返回结果（对话里上传照片时用，要等整理完才能回答）。
// 记录已被后台 worker 领走时，等它整理完；超时返回当前状态，由后台继续。
func (s *Service) ProcessNow(ctx context.Context, familyKey, noteKey string) (Note, error) {
	row, err := s.getRow(ctx, familyKey, noteKey)
	if err != nil {
		return Note{}, err
	}
	if row.ExtractStatus == StatusQueued && row.ClaimedAt == nil {
		now := time.Now()
		claimed := s.db.WithContext(ctx).Model(&noteRow{}).
			Where("id = ? AND extract_status = ? AND claimed_at IS NULL", row.ID, StatusQueued).
			Updates(map[string]any{"extract_status": StatusRunning, "claimed_at": now, "extract_attempts": row.ExtractAttempts + 1})
		if claimed.Error != nil {
			return Note{}, claimed.Error
		}
		if claimed.RowsAffected == 1 {
			// 手机断开也要整理完，结果下次打开还在
			if err := s.finish(context.WithoutCancel(ctx), row); err != nil {
				return Note{}, err
			}
			return s.GetNote(ctx, familyKey, noteKey)
		}
	}
	deadline := time.Now().Add(jobTimeout)
	for time.Now().Before(deadline) {
		note, err := s.GetNote(ctx, familyKey, noteKey)
		if err != nil || note.ExtractStatus == StatusDone || note.ExtractStatus == StatusFailed {
			return note, err
		}
		select {
		case <-ctx.Done():
			return note, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return s.GetNote(ctx, familyKey, noteKey)
}

func (s *Service) safeProcess(ctx context.Context, row noteRow) (result Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("抽取过程出错: %v", recovered)
		}
	}()
	return s.process(ctx, row)
}

func (s *Service) claim(ctx context.Context) (noteRow, bool, error) {
	now := time.Now()
	var row noteRow
	err := s.db.WithContext(ctx).
		Where("(extract_status = ? AND (claimed_at IS NULL OR claimed_at < ?)) OR (extract_status = ? AND claimed_at < ?)",
			StatusQueued, now.Add(-retryBackoff), StatusRunning, now.Add(-staleAfter)).
		Order("id").Limit(1).Find(&row).Error
	if err != nil || row.ID == 0 {
		return noteRow{}, false, err
	}
	// 条件更新抢占：只有状态和领取时间都没被别人改过才算抢到。
	query := s.db.WithContext(ctx).Model(&noteRow{}).Where("id = ? AND extract_status = ?", row.ID, row.ExtractStatus)
	if row.ClaimedAt == nil {
		query = query.Where("claimed_at IS NULL")
	} else {
		query = query.Where("claimed_at = ?", *row.ClaimedAt)
	}
	result := query.Updates(map[string]any{"extract_status": StatusRunning, "claimed_at": now, "extract_attempts": row.ExtractAttempts + 1})
	if result.Error != nil || result.RowsAffected != 1 {
		return noteRow{}, false, result.Error
	}
	return row, true, nil
}
