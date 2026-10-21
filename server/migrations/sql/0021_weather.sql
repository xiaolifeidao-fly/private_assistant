-- 宝宝穿衣（doc/design/10）：
--   1. 每天早 / 中 / 接之前 / 晚的穿衣提醒（推给我和奶奶），存下当时的天气和规则结论；
--   2. 某一天的送接安排临时改（不去幼儿园、晚点接、出去玩）；
--   3. 对话里能查天气、改安排（weather_now / set_outing 工具）。
-- 设置（查哪里的天气、推送时间、送接时间、回家线、推给谁）在 pa_setting（setting_key = weather），没写就用出厂值。

CREATE TABLE IF NOT EXISTS pa_weather_brief (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  brief_key   VARCHAR(40)  NOT NULL,
  family_key  VARCHAR(40)  NOT NULL,
  child_key   VARCHAR(40)  NOT NULL,
  place_code  VARCHAR(16)  NOT NULL DEFAULT '' COMMENT '当时按哪里的天气（中国天气网编码）',
  brief_date  DATE         NOT NULL,
  slot        VARCHAR(16)  NOT NULL COMMENT 'morning / noon / prepickup / evening / alert1 / alert2 …',
  snapshot    JSON         NULL COMMENT '当时用的天气（实况 + 逐小时 + 预警）',
  advice      JSON         NULL COMMENT '规则算出来的结论',
  content     TEXT         NULL COMMENT '推送正文；空 = 正在生成或失败',
  source      VARCHAR(16)  NOT NULL DEFAULT 'llm' COMMENT 'llm / template / skipped（没变化没推）',
  pushed_to   JSON         NULL COMMENT '推给了哪些账号',
  created_by  VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '手动生成的账号；定时为空',
  created_at  DATETIME     NOT NULL,
  updated_at  DATETIME     NOT NULL,
  UNIQUE KEY uk_weather_brief_key (brief_key),
  UNIQUE KEY uk_weather_brief_slot (family_key, child_key, brief_date, slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_weather_day (
  id          BIGINT PRIMARY KEY AUTO_INCREMENT,
  family_key  VARCHAR(40)  NOT NULL,
  child_key   VARCHAR(40)  NOT NULL,
  day         DATE         NOT NULL,
  no_school   TINYINT      NOT NULL DEFAULT 0 COMMENT '这天不去幼儿园',
  drop_at     VARCHAR(5)   NOT NULL DEFAULT '' COMMENT '送的时间 HH:MM，空 = 平常的',
  pickup_at   VARCHAR(5)   NOT NULL DEFAULT '' COMMENT '接的时间',
  home_by     VARCHAR(5)   NOT NULL DEFAULT '' COMMENT '最晚到家',
  outings     JSON         NULL COMMENT '这天出去的时段 [{"from":"15:00","to":"18:00","note":"去外婆家"}]；空 = 平常的',
  note        VARCHAR(200) NOT NULL DEFAULT '',
  updated_by  VARCHAR(40)  NOT NULL DEFAULT '',
  created_at  DATETIME     NOT NULL,
  updated_at  DATETIME     NOT NULL,
  UNIQUE KEY uk_weather_day (family_key, child_key, day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 提醒要写得顺口，用和私人助理同一个对话模型；没有 chat.assistant 路由就不写（只用模板）
INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at)
SELECT 'weather.brief', provider_key, model, 1000, 'low', '宝宝穿衣：把穿衣结论写成家常话', NOW()
  FROM pa_llm_route WHERE task = 'chat.assistant';

-- 已经建好的助理补上 weather_now / set_outing；新库的默认配置会直接带上
UPDATE pa_bot
   SET tools = JSON_ARRAY_APPEND(COALESCE(tools, JSON_ARRAY()), '$', 'weather_now'), updated_at = NOW()
 WHERE bot_key IN ('assistant', 'recall', 'remind') AND (tools IS NULL OR JSON_CONTAINS(tools, JSON_QUOTE('weather_now')) = 0);

UPDATE pa_bot
   SET tools = JSON_ARRAY_APPEND(COALESCE(tools, JSON_ARRAY()), '$', 'set_outing'), updated_at = NOW()
 WHERE bot_key IN ('assistant', 'recall', 'remind') AND (tools IS NULL OR JSON_CONTAINS(tools, JSON_QUOTE('set_outing')) = 0);
