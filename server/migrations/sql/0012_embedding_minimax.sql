-- 向量化接 MiniMax embo-01（1536 维，存入用 db、检索用 query，见 service/llm/minimax.go）。
-- 只在 embedding 路由指向的提供商不存在、且已经配了 minimax 提供商时切过去：已经配好别家向量服务的不动。
-- 切过去后启动时会在后台补算所有记忆的向量（ReindexMissing）。

UPDATE pa_llm_route
   SET provider_key = 'minimax', model = 'embo-01', updated_at = NOW()
 WHERE task = 'embedding'
   AND provider_key NOT IN (SELECT provider_key FROM pa_llm_provider)
   AND EXISTS (SELECT 1 FROM pa_llm_provider WHERE provider_key = 'minimax');
