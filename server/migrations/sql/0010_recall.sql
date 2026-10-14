-- 07 · 万能通：找东西 / 回忆事情 / 拍照记东西。机器人本身由代码写入（bot.EnsureDefaults）。
-- 物品是新的记忆类型 item（attrs.location / usage / photos），不改表结构；照片仍是原始记录的附件（OSS）。

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at) VALUES
  ('chat.recall', 'anthropic', 'claude-sonnet-5-5', 8000, 'low', '万能通对话（找东西、回忆）', NOW());

ALTER TABLE pa_source_note MODIFY channel VARCHAR(20) NOT NULL COMMENT 'quick_note / chat / paste / screenshot / capture / import / manager';
