-- 人设卡只在人设变了时才重写：新记忆进来先用便宜的模型判断「人设变了没有」，变了才走 persona.card 整卡重写。
-- 具体的事、最近的状态留在记忆里，对话时按需检索。

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at) VALUES
  ('persona.gate', 'anthropic', 'claude-sonnet-5-5', 2000, 'low', '人设卡要不要更新（只看新记忆）', NOW());
