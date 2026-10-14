-- M3：人设卡。每个人的卡按版本保存，同一时间只有一个 active；
-- 自动生成和手工修订都是新版本，回滚就是把旧版本重新设为 active。
CREATE TABLE IF NOT EXISTS pa_persona_card (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  card_key       VARCHAR(40)  NOT NULL,
  family_key     VARCHAR(40)  NOT NULL,
  person_key     VARCHAR(40)  NOT NULL,
  version        INT          NOT NULL,
  content        MEDIUMTEXT   NOT NULL COMMENT '人设卡正文（分节的纯文本）',
  changes        TEXT         NULL COMMENT '和上一版相比的变化',
  memory_count   INT          NOT NULL DEFAULT 0 COMMENT '生成时参考的记忆条数',
  memory_cutoff  DATETIME     NULL COMMENT '生成时参考到的最新记忆更新时间',
  status         VARCHAR(20)  NOT NULL COMMENT 'active / history',
  created_by     VARCHAR(20)  NOT NULL COMMENT 'auto / manual / rollback',
  created_at     DATETIME     NOT NULL,
  UNIQUE KEY uk_card_key (card_key),
  UNIQUE KEY uk_person_version (family_key, person_key, version),
  KEY idx_person_status (family_key, person_key, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at) VALUES
  ('persona.card', 'anthropic', 'claude-opus-5-5', 16000, 'high', '人设卡生成（每天按需）', NOW());
