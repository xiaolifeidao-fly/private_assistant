-- 「提醒」助手：说一句要做的事，整理成定时提醒。机器人本身由代码写入（bot.EnsureDefaults）。
-- 用和私人助理同一个对话模型；没有 chat.assistant 路由就不写，需要到管理端「模型路由」里补上。

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at)
SELECT 'chat.remind', provider_key, model, 4000, 'low', '提醒助手（整理要提醒的事和时间）', NOW()
  FROM pa_llm_route WHERE task = 'chat.assistant';
