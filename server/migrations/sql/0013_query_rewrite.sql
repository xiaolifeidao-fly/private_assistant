-- 追问时改写检索词（「那户口本呢」→「户口本放在哪」），见 service/chat/retrieve.go。
-- 用和记忆抽取同一个模型（便宜、会输出 JSON）；没有 memory.extract 路由就不写，改写会自动跳过。

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at)
SELECT 'chat.query', provider_key, model, 1000, 'low', '追问时改写检索词', NOW()
  FROM pa_llm_route WHERE task = 'memory.extract';
