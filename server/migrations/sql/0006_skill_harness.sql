-- 05 · Skill 与 Harness：做事手册、每轮记录、会话分段、评测打分路由。

CREATE TABLE IF NOT EXISTS pa_skill (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  skill_key    VARCHAR(40)  NOT NULL COMMENT 'load_skill 的参数',
  family_key   VARCHAR(40)  NOT NULL,
  name         VARCHAR(40)  NOT NULL,
  when_to_use  VARCHAR(300) NOT NULL COMMENT '一句话：什么时候用，进系统提示词里的目录',
  body         MEDIUMTEXT   NOT NULL COMMENT '正文，模型调用 load_skill 时才读',
  bot_keys     JSON         NULL COMMENT '哪些机器人能用',
  enabled      TINYINT      NOT NULL DEFAULT 1,
  sort         INT          NOT NULL DEFAULT 0,
  version      INT          NOT NULL DEFAULT 1,
  created_at   DATETIME     NOT NULL,
  updated_at   DATETIME     NOT NULL,
  UNIQUE KEY uk_skill_key (skill_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE pa_conversation
  ADD COLUMN segment INT NOT NULL DEFAULT 0 COMMENT '当前分段。历史只回放这一段，超出预算时压缩成摘要开新段' AFTER bot_version;

ALTER TABLE pa_message
  ADD COLUMN segment INT NOT NULL DEFAULT 0 AFTER turn_key,
  ADD COLUMN meta JSON NULL COMMENT '每轮记录：检索、危机信号、引用校验、分段' AFTER `usage`;

-- 助理加上 load_skill；只追加，不覆盖管理端的其他配置。
UPDATE pa_bot
   SET tools = JSON_ARRAY_APPEND(COALESCE(tools, JSON_ARRAY()), '$', 'load_skill'),
       updated_at = NOW()
 WHERE kind = 'assistant'
   AND (tools IS NULL OR JSON_CONTAINS(tools, JSON_QUOTE('load_skill')) = 0);

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at) VALUES
  ('chat.summary', 'anthropic', 'claude-sonnet-5-5', 8000, NULL,     '长对话分段摘要', NOW()),
  ('eval.judge',   'anthropic', 'claude-sonnet-5-5', 8000, 'medium', '评测打分',       NOW());
