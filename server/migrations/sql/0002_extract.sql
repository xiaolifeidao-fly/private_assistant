-- M2：原始记录的自动抽取。pa_source_note 本身就是任务队列（按 extract_status 抢占），
-- 进程重启不丢任务，不额外依赖 Redis。
ALTER TABLE pa_source_note
  ADD COLUMN extract_attempts INT NOT NULL DEFAULT 0 AFTER extract_status,
  ADD COLUMN claimed_at DATETIME NULL COMMENT '被 worker 领取的时间；失败重试时也用它做退避' AFTER extract_attempts,
  ADD COLUMN extracted_at DATETIME NULL AFTER claimed_at,
  ADD COLUMN extract_result JSON NULL COMMENT '抽取结果摘要：新增/待确认/更新/重复的记忆编号、认不出的人' AFTER extracted_at,
  ADD COLUMN extract_error VARCHAR(1000) NULL AFTER extract_result,
  ADD KEY idx_extract (extract_status, claimed_at);

ALTER TABLE pa_memory ADD KEY idx_source_note (source_note_key);
