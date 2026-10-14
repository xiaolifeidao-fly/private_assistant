-- 06 · 越用越准：可配置的对话规则；对话自动抽取记忆的进度；会话冻结时的人设卡版本。

CREATE TABLE IF NOT EXISTS pa_setting (
  setting_key  VARCHAR(40) NOT NULL PRIMARY KEY COMMENT 'safety / budget / learning',
  value        JSON        NOT NULL COMMENT '只存改过的字段也行，读的时候和出厂值合并',
  updated_at   DATETIME    NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE pa_conversation
  ADD COLUMN extracted_message_id BIGINT NOT NULL DEFAULT 0 COMMENT '自动抽取记忆已处理到哪条消息（pa_message.id）' AFTER segment,
  ADD COLUMN persona_versions JSON NULL COMMENT '冻结系统提示词时各人人设卡的版本，之后有新版本就在每轮参考里提示变化' AFTER extracted_message_id;

-- 分身也能读做事手册（设计文档 06 · 0 #4）。只追加，不覆盖管理端的其他配置。
UPDATE pa_bot
   SET tools = JSON_ARRAY_APPEND(COALESCE(tools, JSON_ARRAY()), '$', 'load_skill'),
       updated_at = NOW()
 WHERE kind = 'persona'
   AND (tools IS NULL OR JSON_CONTAINS(tools, JSON_QUOTE('load_skill')) = 0);
