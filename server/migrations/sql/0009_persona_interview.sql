-- 对话补充人设卡：我和模型聊，模型挑信息不够的地方问；整理草稿复用 persona.card 路由。
-- 存下来的版本 created_by = chat。

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at) VALUES
  ('persona.interview', 'anthropic', 'claude-sonnet-5-5', 8000, 'medium', '对话补充人设卡（提问）', NOW());

ALTER TABLE pa_persona_card MODIFY created_by VARCHAR(20) NOT NULL COMMENT 'auto / manual / rollback / chat';
