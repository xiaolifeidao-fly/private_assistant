-- M4：主动提醒、回老家行程与 Web Push。
CREATE TABLE IF NOT EXISTS pa_reminder (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  reminder_key   VARCHAR(40)  NOT NULL,
  family_key     VARCHAR(40)  NOT NULL,
  kind           VARCHAR(30)  NOT NULL COMMENT 'manual / birthday / commitment / dated / festival / trip / daily',
  title          VARCHAR(200) NOT NULL,
  body           TEXT         NULL,
  event_at       DATETIME     NULL,
  remind_at      DATETIME     NOT NULL,
  status         VARCHAR(20)  NOT NULL COMMENT 'scheduled / sending / sent / failed / dismissed / cancelled',
  source_type    VARCHAR(30)  NULL,
  source_key     VARCHAR(60)  NULL,
  dedupe_key     VARCHAR(160) NOT NULL,
  target_url     VARCHAR(500) NULL,
  payload        JSON         NULL,
  attempts       INT          NOT NULL DEFAULT 0,
  last_error     VARCHAR(1000) NULL,
  sent_at        DATETIME     NULL,
  created_at     DATETIME     NOT NULL,
  updated_at     DATETIME     NOT NULL,
  UNIQUE KEY uk_reminder_key (reminder_key),
  UNIQUE KEY uk_reminder_dedupe (family_key, dedupe_key),
  KEY idx_reminder_due (family_key, status, remind_at),
  KEY idx_reminder_event (family_key, event_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_trip (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  trip_key       VARCHAR(40)  NOT NULL,
  family_key     VARCHAR(40)  NOT NULL,
  place_key      VARCHAR(40)  NOT NULL,
  title          VARCHAR(200) NOT NULL,
  start_at       DATETIME     NOT NULL,
  end_at         DATETIME     NULL,
  people         JSON         NULL COMMENT '预计会见到的 person_key',
  notes          TEXT         NULL,
  status         VARCHAR(20)  NOT NULL DEFAULT 'planned' COMMENT 'planned / done / cancelled',
  created_at     DATETIME     NOT NULL,
  updated_at     DATETIME     NOT NULL,
  UNIQUE KEY uk_trip_key (trip_key),
  KEY idx_trip_time (family_key, status, start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_push_subscription (
  id               BIGINT PRIMARY KEY AUTO_INCREMENT,
  subscription_key VARCHAR(40)   NOT NULL,
  family_key       VARCHAR(40)   NOT NULL,
  account_key      VARCHAR(40)   NOT NULL,
  endpoint_hash    CHAR(64)      NOT NULL,
  endpoint         VARCHAR(1500) NOT NULL,
  p256dh           VARCHAR(500)  NOT NULL,
  auth             VARCHAR(500)  NOT NULL,
  user_agent       VARCHAR(500)  NULL,
  enabled          TINYINT       NOT NULL DEFAULT 1,
  last_success_at  DATETIME      NULL,
  last_error       VARCHAR(1000) NULL,
  created_at       DATETIME      NOT NULL,
  updated_at       DATETIME      NOT NULL,
  UNIQUE KEY uk_push_subscription (subscription_key),
  UNIQUE KEY uk_push_endpoint (family_key, endpoint_hash),
  KEY idx_push_account (family_key, account_key, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_push_delivery (
  id               BIGINT PRIMARY KEY AUTO_INCREMENT,
  delivery_key     VARCHAR(40)   NOT NULL,
  family_key       VARCHAR(40)   NOT NULL,
  reminder_key     VARCHAR(40)   NOT NULL,
  subscription_key VARCHAR(40)   NOT NULL,
  status           VARCHAR(20)   NOT NULL COMMENT 'sent / failed',
  http_status      INT           NOT NULL DEFAULT 0,
  error            VARCHAR(1000) NULL,
  created_at       DATETIME      NOT NULL,
  UNIQUE KEY uk_delivery_key (delivery_key),
  KEY idx_delivery_reminder (family_key, reminder_key, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 升级已经创建过的助理；新库的默认配置会直接带上这两个工具。
UPDATE pa_bot
   SET tools = JSON_ARRAY_APPEND(COALESCE(tools, JSON_ARRAY()), '$', 'list_upcoming'), updated_at = NOW()
 WHERE bot_key = 'assistant' AND (tools IS NULL OR JSON_CONTAINS(tools, JSON_QUOTE('list_upcoming')) = 0);

UPDATE pa_bot
   SET tools = JSON_ARRAY_APPEND(COALESCE(tools, JSON_ARRAY()), '$', 'create_reminder'), updated_at = NOW()
 WHERE bot_key = 'assistant' AND (tools IS NULL OR JSON_CONTAINS(tools, JSON_QUOTE('create_reminder')) = 0);
