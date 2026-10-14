-- 06 · 越用越准（M5）：手册改写、手册示例、周洞察、记忆合并建议。

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at) VALUES
  ('skill.rewrite', 'anthropic', 'claude-opus-5-5', 16000, 'high', '按家里的记忆改写手册', NOW());

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at) VALUES
  ('skill.example', 'anthropic', 'claude-sonnet-5-5', 8000,  'medium', '从点踩反馈整理话术示例', NOW()),
  ('insight.weekly', 'anthropic', 'claude-opus-5-5',  16000, 'high',   '每周洞察',               NOW());

-- 手册示例：点踩 + 修正自动生成待审示例，审过的随手册正文一起给模型看（few-shot）。
CREATE TABLE IF NOT EXISTS pa_skill_example (
  id                  BIGINT PRIMARY KEY AUTO_INCREMENT,
  example_key         VARCHAR(40)  NOT NULL,
  family_key          VARCHAR(40)  NOT NULL,
  skill_key           VARCHAR(40)  NOT NULL,
  situation           TEXT         NOT NULL COMMENT '具体情况',
  good_response       TEXT         NOT NULL COMMENT '好的说法/做法',
  bad_response        TEXT         NULL COMMENT '反面示例',
  why                 TEXT         NULL COMMENT '背后的原则',
  status              VARCHAR(20)  NOT NULL DEFAULT 'pending' COMMENT 'pending / approved / rejected',
  source              VARCHAR(20)  NOT NULL DEFAULT 'manual' COMMENT 'manual / feedback',
  source_message_key  VARCHAR(40)  NULL,
  created_at          DATETIME     NOT NULL,
  updated_at          DATETIME     NOT NULL,
  UNIQUE KEY uk_example_key (example_key),
  KEY idx_skill_status (family_key, skill_key, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 疑似重复的记忆：每周按向量相似度找出来，由我决定合并（保留一条，另一条归档）还是忽略。
CREATE TABLE IF NOT EXISTS pa_memory_merge (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  merge_key    VARCHAR(40)  NOT NULL,
  family_key   VARCHAR(40)  NOT NULL,
  a_key        VARCHAR(40)  NOT NULL,
  b_key        VARCHAR(40)  NOT NULL,
  similarity   DOUBLE       NOT NULL,
  status       VARCHAR(20)  NOT NULL DEFAULT 'pending' COMMENT 'pending / merged / dismissed',
  kept_key     VARCHAR(40)  NULL,
  created_at   DATETIME     NOT NULL,
  decided_at   DATETIME     NULL,
  UNIQUE KEY uk_merge_key (merge_key),
  UNIQUE KEY uk_pair (family_key, a_key, b_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 定时任务的执行记录：同一个周期只跑一次（两个进程都跑也不重复）。
CREATE TABLE IF NOT EXISTS pa_job_run (
  job         VARCHAR(40)  NOT NULL,
  period      VARCHAR(40)  NOT NULL COMMENT '如 2026-W41',
  family_key  VARCHAR(40)  NOT NULL,
  result      TEXT         NULL,
  created_at  DATETIME     NOT NULL,
  PRIMARY KEY (job, period, family_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
