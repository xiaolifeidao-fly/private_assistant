-- 对话里说话：录音交给语音识别转成文字，再发给模型。默认用 MiniMax asr-1.0，和对话共用 minimax 提供商的 Key
-- （地址带 minimax 时自动换成 /v1/speech_to_text）。没有 minimax 提供商的，到管理端「模型」里把 asr 指向别的
-- OpenAI 兼容 /audio/transcriptions 服务。

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at) VALUES
  ('asr', 'minimax', 'asr-1.0', 0, NULL, '语音转文字（对话、随手录里说话）', NOW());
