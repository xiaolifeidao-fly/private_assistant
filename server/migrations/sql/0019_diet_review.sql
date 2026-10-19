-- 孩子饮食（doc/design/09）续：
--   1. 每周计划对照实际的点评（周日晚上自动生成，推给我和老婆；页面上也能手动生成）；
--   2. 对话里的助理能查饮食（diet_overview 工具）。

CREATE TABLE IF NOT EXISTS pa_diet_review (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  review_key     VARCHAR(40)  NOT NULL,
  family_key     VARCHAR(40)  NOT NULL,
  child_key      VARCHAR(40)  NOT NULL,
  week_start     DATE         NOT NULL COMMENT '这一周的周一',
  content        TEXT         NULL COMMENT '点评正文；空表示正在生成或生成失败',
  created_by     VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '手动生成的账号；定时生成为空',
  created_at     DATETIME     NOT NULL,
  updated_at     DATETIME     NOT NULL,
  UNIQUE KEY uk_diet_review_key (review_key),
  UNIQUE KEY uk_diet_review_week (family_key, child_key, week_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 点评要写得通顺，用和私人助理同一个对话模型；没有 chat.assistant 路由就不写
INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at)
SELECT 'diet.review', provider_key, model, 3000, 'low', '孩子饮食：每周计划对照实际', NOW()
  FROM pa_llm_route WHERE task = 'chat.assistant';

-- 已经建好的助理补上 diet_overview；新库的默认配置会直接带上
UPDATE pa_bot
   SET tools = JSON_ARRAY_APPEND(COALESCE(tools, JSON_ARRAY()), '$', 'diet_overview'), updated_at = NOW()
 WHERE bot_key IN ('assistant', 'recall', 'daughter') AND (tools IS NULL OR JSON_CONTAINS(tools, JSON_QUOTE('diet_overview')) = 0);
