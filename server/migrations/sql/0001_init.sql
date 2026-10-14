-- M0 / M1 基础表。表前缀 pa_；跨表引用一律用字符串业务键，不建外键。

-- ---------------------------------------------------------------- 账号
CREATE TABLE IF NOT EXISTS pa_family (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  family_key  VARCHAR(40)  NOT NULL,
  name        VARCHAR(100) NOT NULL,
  created_at  DATETIME     NOT NULL,
  updated_at  DATETIME     NOT NULL,
  UNIQUE KEY uk_family_key (family_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_account (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  account_key    VARCHAR(40)  NOT NULL,
  realm          VARCHAR(20)  NOT NULL COMMENT 'manager=管理端 app=用户端，两套互不相通',
  username       VARCHAR(60)  NOT NULL,
  password_hash  VARCHAR(100) NOT NULL,
  display_name   VARCHAR(60)  NOT NULL,
  role           VARCHAR(20)  NOT NULL DEFAULT 'member' COMMENT 'admin / member',
  family_key     VARCHAR(40)  NOT NULL,
  person_key     VARCHAR(40)  NULL COMMENT 'app 账号对应家里哪个人（一般是 me）',
  status         VARCHAR(20)  NOT NULL DEFAULT 'active',
  last_login_at  DATETIME     NULL,
  created_at     DATETIME     NOT NULL,
  updated_at     DATETIME     NOT NULL,
  UNIQUE KEY uk_account_key (account_key),
  UNIQUE KEY uk_realm_username (realm, username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_session (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  token_hash   CHAR(64)    NOT NULL COMMENT '令牌的 sha256，库里不存明文',
  account_key  VARCHAR(40) NOT NULL,
  realm        VARCHAR(20) NOT NULL,
  client_ip    VARCHAR(64) NULL,
  user_agent   VARCHAR(300) NULL,
  expires_at   DATETIME    NOT NULL,
  created_at   DATETIME    NOT NULL,
  last_seen_at DATETIME    NOT NULL,
  UNIQUE KEY uk_token_hash (token_hash),
  KEY idx_account (account_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_login_record (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  realm       VARCHAR(20)  NOT NULL,
  username    VARCHAR(60)  NOT NULL,
  success     TINYINT      NOT NULL,
  reason      VARCHAR(100) NULL,
  client_ip   VARCHAR(64)  NULL,
  created_at  DATETIME     NOT NULL,
  KEY idx_realm_user_time (realm, username, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ---------------------------------------------------------------- 家庭档案
CREATE TABLE IF NOT EXISTS pa_person (
  id              BIGINT PRIMARY KEY AUTO_INCREMENT,
  person_key      VARCHAR(40)  NOT NULL,
  family_key      VARCHAR(40)  NOT NULL,
  name            VARCHAR(60)  NOT NULL COMMENT '真名或常用名',
  role_label      VARCHAR(40)  NOT NULL COMMENT '在这个家里的身份：老婆、妈妈、大舅……',
  aliases         JSON         NULL COMMENT '别名，抽取时用来认人：["她","老婆","媳妇"]',
  gender          VARCHAR(10)  NULL,
  birthday        DATE         NULL COMMENT '公历生日',
  lunar_birthday  VARCHAR(10)  NULL COMMENT '农历生日 MM-DD，闰月写 L07-15',
  birthday_by     VARCHAR(10)  NOT NULL DEFAULT 'solar' COMMENT '过哪个生日：solar / lunar',
  home_place_key  VARCHAR(40)  NULL,
  side            VARCHAR(20)  NOT NULL DEFAULT 'core' COMMENT 'core 核心小家 / self 我这边 / wife 老婆那边',
  importance      TINYINT      NOT NULL DEFAULT 3,
  is_core         TINYINT      NOT NULL DEFAULT 0 COMMENT '核心成员（有分身的人和我）',
  profile         TEXT         NULL COMMENT '自由描述：职业、性格概况……',
  avatar          VARCHAR(300) NULL,
  sort            INT          NOT NULL DEFAULT 0,
  status          VARCHAR(20)  NOT NULL DEFAULT 'active',
  created_at      DATETIME     NOT NULL,
  updated_at      DATETIME     NOT NULL,
  UNIQUE KEY uk_person_key (person_key),
  KEY idx_family (family_key, status, sort)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_person_relation (
  id                BIGINT PRIMARY KEY AUTO_INCREMENT,
  relation_key      VARCHAR(40)  NOT NULL,
  family_key        VARCHAR(40)  NOT NULL,
  from_person       VARCHAR(40)  NOT NULL,
  to_person         VARCHAR(40)  NOT NULL,
  relation          VARCHAR(40)  NOT NULL COMMENT 'to_person 是 from_person 的什么人：父亲、大舅……',
  address_as        VARCHAR(40)  NULL COMMENT 'from_person 怎么称呼 to_person',
  child_address_as  VARCHAR(40)  NULL COMMENT '女儿怎么称呼 to_person',
  closeness         TINYINT      NOT NULL DEFAULT 3,
  notes             TEXT         NULL COMMENT '关系要点、历史渊源',
  sensitivities     TEXT         NULL COMMENT '雷区',
  created_at        DATETIME     NOT NULL,
  updated_at        DATETIME     NOT NULL,
  UNIQUE KEY uk_relation_key (relation_key),
  KEY idx_from (family_key, from_person),
  KEY idx_to (family_key, to_person)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_place (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  place_key    VARCHAR(40)  NOT NULL,
  family_key   VARCHAR(40)  NOT NULL,
  name         VARCHAR(60)  NOT NULL,
  kind         VARCHAR(20)  NOT NULL DEFAULT 'home' COMMENT 'home 现在住的家 / hometown 老家 / other',
  address      VARCHAR(200) NULL,
  dialect      VARCHAR(40)  NULL,
  description  TEXT         NULL,
  customs      TEXT         NULL COMMENT '规矩习俗概要；细项在记忆里（type=custom）',
  travel_tips  TEXT         NULL,
  sort         INT          NOT NULL DEFAULT 0,
  created_at   DATETIME     NOT NULL,
  updated_at   DATETIME     NOT NULL,
  UNIQUE KEY uk_place_key (place_key),
  KEY idx_family (family_key, sort)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ---------------------------------------------------------------- 记忆
CREATE TABLE IF NOT EXISTS pa_source_note (
  id              BIGINT PRIMARY KEY AUTO_INCREMENT,
  note_key        VARCHAR(40)  NOT NULL,
  family_key      VARCHAR(40)  NOT NULL,
  channel         VARCHAR(20)  NOT NULL COMMENT 'quick_note / chat / paste / screenshot / voice / manager',
  raw_text        MEDIUMTEXT   NOT NULL COMMENT '原文，只增不改',
  attachments     JSON         NULL,
  about_persons   JSON         NULL,
  occurred_at     DATETIME     NULL COMMENT '事情发生的时间（不是录入时间）',
  extract_status  VARCHAR(20)  NOT NULL DEFAULT 'none' COMMENT 'none / queued / done / failed',
  created_by      VARCHAR(40)  NOT NULL,
  created_at      DATETIME     NOT NULL,
  UNIQUE KEY uk_note_key (note_key),
  KEY idx_family_time (family_key, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_memory (
  id               BIGINT PRIMARY KEY AUTO_INCREMENT,
  memory_key       VARCHAR(40)   NOT NULL,
  family_key       VARCHAR(40)   NOT NULL,
  type             VARCHAR(20)   NOT NULL,
  scope            VARCHAR(20)   NOT NULL DEFAULT 'person' COMMENT 'person / shared / place / relation',
  subject_person   VARCHAR(40)   NULL COMMENT '主要关于谁',
  place_key        VARCHAR(40)   NULL,
  title            VARCHAR(200)  NOT NULL,
  content          TEXT          NOT NULL,
  attrs            JSON          NULL,
  importance       TINYINT       NOT NULL DEFAULT 3,
  confidence       DECIMAL(3,2)  NOT NULL DEFAULT 1.00,
  status           VARCHAR(20)   NOT NULL DEFAULT 'active' COMMENT 'pending / active / superseded / archived / rejected',
  occurred_at      DATETIME      NULL COMMENT '事件/原话发生时间',
  valid_from       DATETIME      NULL,
  valid_to         DATETIME      NULL,
  supersedes_key   VARCHAR(40)   NULL,
  source_note_key  VARCHAR(40)   NULL,
  created_by       VARCHAR(20)   NOT NULL COMMENT 'manual / chat / extract / reflect',
  use_count        INT           NOT NULL DEFAULT 0,
  last_used_at     DATETIME      NULL,
  created_at       DATETIME      NOT NULL,
  updated_at       DATETIME      NOT NULL,
  UNIQUE KEY uk_memory_key (memory_key),
  KEY idx_family_subject (family_key, subject_person, type, status),
  KEY idx_family_status (family_key, status, importance),
  FULLTEXT KEY ft_memory (title, content) WITH PARSER ngram
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_memory_link (
  id           BIGINT PRIMARY KEY AUTO_INCREMENT,
  memory_key   VARCHAR(40) NOT NULL,
  target_type  VARCHAR(10) NOT NULL COMMENT 'person / place',
  target_key   VARCHAR(40) NOT NULL,
  UNIQUE KEY uk_link (memory_key, target_type, target_key),
  KEY idx_target (target_type, target_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_memory_embedding (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  memory_key  VARCHAR(40)  NOT NULL,
  family_key  VARCHAR(40)  NOT NULL,
  model       VARCHAR(80)  NOT NULL COMMENT '换 embedding 模型必须全量重算',
  dim         INT          NOT NULL,
  `vector`    MEDIUMBLOB   NOT NULL COMMENT 'float32 小端序（VECTOR 在 MySQL 9 是保留字，必须加反引号）',
  updated_at  DATETIME     NOT NULL,
  UNIQUE KEY uk_memory_key (memory_key),
  KEY idx_family_model (family_key, model)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ---------------------------------------------------------------- 机器人与对话
CREATE TABLE IF NOT EXISTS pa_bot (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  bot_key        VARCHAR(40)  NOT NULL,
  family_key     VARCHAR(40)  NOT NULL,
  name           VARCHAR(40)  NOT NULL,
  kind           VARCHAR(20)  NOT NULL COMMENT 'assistant / persona',
  person_key     VARCHAR(40)  NULL COMMENT 'persona 绑定的人',
  avatar         VARCHAR(300) NULL,
  description    VARCHAR(300) NULL,
  greeting       VARCHAR(500) NULL,
  system_prompt  MEDIUMTEXT   NOT NULL,
  route_task     VARCHAR(40)  NOT NULL COMMENT '用哪条模型路由（pa_llm_route.task）',
  tools          JSON         NULL COMMENT '开启的工具名列表',
  memory_scope   VARCHAR(20)  NOT NULL DEFAULT 'all' COMMENT 'all 全部 / person 仅本人+公共',
  enabled        TINYINT      NOT NULL DEFAULT 1,
  sort           INT          NOT NULL DEFAULT 0,
  version        INT          NOT NULL DEFAULT 1,
  created_at     DATETIME     NOT NULL,
  updated_at     DATETIME     NOT NULL,
  UNIQUE KEY uk_bot_key (bot_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_conversation (
  id                BIGINT PRIMARY KEY AUTO_INCREMENT,
  conversation_key  VARCHAR(40)  NOT NULL,
  family_key        VARCHAR(40)  NOT NULL,
  account_key       VARCHAR(40)  NOT NULL,
  bot_key           VARCHAR(40)  NOT NULL,
  title             VARCHAR(200) NOT NULL,
  status            VARCHAR(20)  NOT NULL DEFAULT 'active',
  message_count     INT          NOT NULL DEFAULT 0,
  system_snapshot   MEDIUMTEXT   NULL COMMENT '会话开始时冻结的系统提示词。只追加不改历史：中途改它会让 Claude 之前的 thinking 块失效、提示词缓存全部重来',
  tools_snapshot    JSON         NULL COMMENT '会话开始时冻结的工具名列表，理由同上',
  bot_version       INT          NOT NULL DEFAULT 1,
  last_message_at   DATETIME     NULL,
  created_at        DATETIME     NOT NULL,
  updated_at        DATETIME     NOT NULL,
  UNIQUE KEY uk_conversation_key (conversation_key),
  KEY idx_account_bot (account_key, bot_key, status, last_message_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_message (
  id                BIGINT PRIMARY KEY AUTO_INCREMENT,
  message_key       VARCHAR(40)  NOT NULL,
  conversation_key  VARCHAR(40)  NOT NULL,
  turn_key          VARCHAR(40)  NOT NULL COMMENT '一次提问及其全部工具往返共用一个 turn',
  role              VARCHAR(20)  NOT NULL COMMENT 'user / assistant / tool',
  content           MEDIUMTEXT   NOT NULL COMMENT '展示用纯文本',
  blocks            JSON         NULL COMMENT '与提供商无关的内容块（文本、工具调用、工具结果）',
  provider          VARCHAR(40)  NULL,
  model             VARCHAR(80)  NULL,
  raw               JSON         NULL COMMENT '提供商原生的 assistant 内容（含 thinking 签名），同提供商续聊时原样回传',
  `usage`           JSON         NULL COMMENT 'USAGE 是 MySQL 保留字，必须加反引号',
  status            VARCHAR(20)  NOT NULL DEFAULT 'done' COMMENT 'done / error',
  error             VARCHAR(1000) NULL,
  feedback          VARCHAR(10)  NULL COMMENT 'up / down',
  feedback_note     TEXT         NULL,
  created_at        DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_message_key (message_key),
  KEY idx_conversation (conversation_key, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ---------------------------------------------------------------- 模型网关
CREATE TABLE IF NOT EXISTS pa_llm_provider (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  provider_key  VARCHAR(40)  NOT NULL,
  name          VARCHAR(60)  NOT NULL,
  kind          VARCHAR(20)  NOT NULL COMMENT 'anthropic / openai（OpenAI 兼容协议：DeepSeek、通义、豆包、Ollama……）',
  base_url      VARCHAR(300) NULL,
  api_key       VARCHAR(500) NOT NULL DEFAULT '',
  enabled       TINYINT      NOT NULL DEFAULT 1,
  created_at    DATETIME     NOT NULL,
  updated_at    DATETIME     NOT NULL,
  UNIQUE KEY uk_provider_key (provider_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_llm_route (
  id            BIGINT PRIMARY KEY AUTO_INCREMENT,
  task          VARCHAR(40)  NOT NULL,
  provider_key  VARCHAR(40)  NOT NULL,
  model         VARCHAR(80)  NOT NULL,
  max_tokens    INT          NOT NULL DEFAULT 16000,
  effort        VARCHAR(10)  NULL COMMENT 'Claude 的 output_config.effort',
  temperature   DECIMAL(3,2) NULL COMMENT '只对支持采样参数的模型生效',
  description   VARCHAR(200) NULL,
  updated_at    DATETIME     NOT NULL,
  UNIQUE KEY uk_task (task)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_llm_call (
  id                BIGINT PRIMARY KEY AUTO_INCREMENT,
  call_key          VARCHAR(40)  NOT NULL,
  task              VARCHAR(40)  NOT NULL,
  provider_key      VARCHAR(40)  NOT NULL,
  model             VARCHAR(80)  NOT NULL,
  ref_key           VARCHAR(40)  NULL COMMENT '关联的会话或记忆',
  input_tokens      INT          NOT NULL DEFAULT 0,
  output_tokens     INT          NOT NULL DEFAULT 0,
  cache_read_tokens INT          NOT NULL DEFAULT 0,
  cache_write_tokens INT         NOT NULL DEFAULT 0,
  latency_ms        INT          NOT NULL DEFAULT 0,
  stop_reason       VARCHAR(40)  NULL,
  status            VARCHAR(20)  NOT NULL,
  error             VARCHAR(1000) NULL,
  created_at        DATETIME     NOT NULL,
  UNIQUE KEY uk_call_key (call_key),
  KEY idx_time (created_at),
  KEY idx_ref (ref_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ---------------------------------------------------------------- 初始数据
-- 名字先用称呼占位，到管理端「家庭档案」里改成真实信息。
INSERT IGNORE INTO pa_family (family_key, name, created_at, updated_at)
VALUES ('default', '我们家', NOW(), NOW());

INSERT IGNORE INTO pa_place (place_key, family_key, name, kind, sort, created_at, updated_at) VALUES
  ('home_shanghai',     'default', '上海的家',     'home',     1, NOW(), NOW()),
  ('home_my_parents',   'default', '我爸妈老家',   'hometown', 2, NOW(), NOW()),
  ('home_wife_parents', 'default', '岳父母老家',   'hometown', 3, NOW(), NOW());

INSERT IGNORE INTO pa_person (person_key, family_key, name, role_label, aliases, gender, home_place_key, side, importance, is_core, sort, created_at, updated_at) VALUES
  ('me',          'default', '我',   '我',     JSON_ARRAY('我','老公','爸爸'),               'male',   'home_shanghai',     'core', 5, 1, 1, NOW(), NOW()),
  ('wife',        'default', '老婆', '老婆',   JSON_ARRAY('她','老婆','媳妇','孩子妈'),      'female', 'home_shanghai',     'core', 5, 1, 2, NOW(), NOW()),
  ('mom',         'default', '妈妈', '我妈',   JSON_ARRAY('我妈','妈','老妈','奶奶','婆婆'), 'female', 'home_shanghai',     'core', 5, 1, 3, NOW(), NOW()),
  ('daughter',    'default', '女儿', '女儿',   JSON_ARRAY('女儿','闺女','宝宝','孩子'),      'female', 'home_shanghai',     'core', 5, 1, 4, NOW(), NOW()),
  ('dad',         'default', '爸爸', '我爸',   JSON_ARRAY('我爸','爸','爷爷','公公'),        'male',   'home_my_parents',   'self', 4, 0, 5, NOW(), NOW()),
  ('wife_father', 'default', '岳父', '岳父',   JSON_ARRAY('岳父','老丈人','外公'),           'male',   'home_wife_parents', 'wife', 4, 0, 6, NOW(), NOW()),
  ('wife_mother', 'default', '岳母', '岳母',   JSON_ARRAY('岳母','丈母娘','外婆'),           'female', 'home_wife_parents', 'wife', 4, 0, 7, NOW(), NOW());

INSERT IGNORE INTO pa_person_relation (relation_key, family_key, from_person, to_person, relation, address_as, child_address_as, closeness, created_at, updated_at) VALUES
  ('rel_me_wife',          'default', 'me', 'wife',        '妻子',   NULL,   '妈妈', 5, NOW(), NOW()),
  ('rel_me_mom',           'default', 'me', 'mom',         '母亲',   '妈',   '奶奶', 5, NOW(), NOW()),
  ('rel_me_daughter',      'default', 'me', 'daughter',    '女儿',   NULL,   NULL,   5, NOW(), NOW()),
  ('rel_me_dad',           'default', 'me', 'dad',         '父亲',   '爸',   '爷爷', 4, NOW(), NOW()),
  ('rel_me_wife_father',   'default', 'me', 'wife_father', '岳父',   '爸',   '外公', 4, NOW(), NOW()),
  ('rel_me_wife_mother',   'default', 'me', 'wife_mother', '岳母',   '妈',   '外婆', 4, NOW(), NOW()),
  ('rel_wife_mom',         'default', 'wife', 'mom',       '婆婆',   '妈',   NULL,   3, NOW(), NOW());

-- 模型路由：提供商（含 API Key）在管理端「模型」里配置，provider_key 对上即可生效。
INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at) VALUES
  ('chat.assistant', 'anthropic', 'claude-opus-5-5',   32000, 'high',   '私人助理对话', NOW()),
  ('chat.persona',   'anthropic', 'claude-sonnet-5-5', 16000, 'medium', '分身对话',     NOW()),
  ('memory.extract', 'anthropic', 'claude-haiku-4-5',  8000,  NULL,     '记忆抽取（M2）', NOW()),
  ('embedding',      'embedding', 'text-embedding-v3', 0,     NULL,     '向量化（OpenAI 兼容 /embeddings）', NOW());
