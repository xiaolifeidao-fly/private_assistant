-- 检索结果重排：先宽一点捞 20 条候选，由模型挑出真正相关的并排序（service/chat/retrieve.go）。
-- 用和记忆抽取同一个模型；没有这条路由就只用分数门槛。

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at)
SELECT 'chat.rerank', provider_key, model, 1000, 'low', '检索结果重排', NOW()
  FROM pa_llm_route WHERE task = 'memory.extract';
