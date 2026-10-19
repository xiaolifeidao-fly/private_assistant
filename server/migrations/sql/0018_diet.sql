-- 孩子的饮食（doc/design/09）：两张表，一张是计划，一张是实际吃了什么。
--   pa_diet_plan    预先计划：哪天哪一餐打算吃什么、有什么要求。我和老婆的账号能改，奶奶的账号只看。
--   pa_diet_record  实际发生：谁什么时候喂了什么、吃了多少。谁都能记，奶奶记得最多（拍照、说一句就行）。
-- 全家共用，不分私有（不进 common/viewer 的受保护表）。删掉是软删（status=deleted），留痕。

CREATE TABLE IF NOT EXISTS pa_diet_plan (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  plan_key       VARCHAR(40)  NOT NULL,
  family_key     VARCHAR(40)  NOT NULL,
  child_key      VARCHAR(40)  NOT NULL COMMENT '哪个孩子（pa_person.person_key）',
  plan_date      DATE         NOT NULL,
  meal           VARCHAR(20)  NOT NULL COMMENT 'day 全天要求 / breakfast / morning_snack / lunch / afternoon_snack / dinner / night_snack',
  foods          VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '打算吃什么',
  note           VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '要求、注意（少盐、不给糖……）',
  status         VARCHAR(20)  NOT NULL DEFAULT 'active' COMMENT 'active / deleted',
  created_by     VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '账号 account_key',
  updated_by     VARCHAR(40)  NOT NULL DEFAULT '',
  created_at     DATETIME     NOT NULL,
  updated_at     DATETIME     NOT NULL,
  UNIQUE KEY uk_diet_plan_key (plan_key),
  KEY idx_diet_plan_day (family_key, child_key, plan_date, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pa_diet_record (
  id             BIGINT PRIMARY KEY AUTO_INCREMENT,
  record_key     VARCHAR(40)  NOT NULL,
  family_key     VARCHAR(40)  NOT NULL,
  child_key      VARCHAR(40)  NOT NULL,
  eaten_at       DATETIME     NOT NULL COMMENT '什么时候吃的',
  meal           VARCHAR(20)  NOT NULL COMMENT '同 pa_diet_plan.meal（没有 day）',
  foods          VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '实际吃了什么',
  amount         VARCHAR(20)  NOT NULL DEFAULT '' COMMENT 'all 吃完 / most 大半 / half 一半 / little 几口 / none 没吃 / 空串 没说',
  note           VARCHAR(1000) NOT NULL DEFAULT '' COMMENT '情况：挑食、哭闹、加餐的原因……',
  source         VARCHAR(20)  NOT NULL DEFAULT 'manual' COMMENT 'manual 手填 / capture 拍照或说一句、模型整理',
  raw_text       TEXT         NULL COMMENT 'capture 时说的原话（语音转的文字）',
  photos         JSON         NULL COMMENT '照片 [{path, mediaType, size}]',
  capture_key    VARCHAR(40)  NOT NULL DEFAULT '' COMMENT '同一次拍照/说话整理出的几条共用',
  status         VARCHAR(20)  NOT NULL DEFAULT 'active' COMMENT 'active / deleted',
  created_by     VARCHAR(40)  NOT NULL DEFAULT '',
  updated_by     VARCHAR(40)  NOT NULL DEFAULT '',
  created_at     DATETIME     NOT NULL,
  updated_at     DATETIME     NOT NULL,
  UNIQUE KEY uk_diet_record_key (record_key),
  KEY idx_diet_record_day (family_key, child_key, eaten_at, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 拍照/说一句整理成饮食记录：要看图，用和记忆抽取同一个模型；没有 memory.extract 路由就不写，
-- 整理不了时原话照样记下来（见 service/diet）。
INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at)
SELECT 'diet.extract', provider_key, model, 4000, NULL, '孩子饮食：拍照/说一句整理成记录', NOW()
  FROM pa_llm_route WHERE task = 'memory.extract';

-- 说一句 / 拍张食谱改计划（我和老婆用）：排一周的菜输出多一些
INSERT IGNORE INTO pa_llm_route (task, provider_key, model, max_tokens, effort, description, updated_at)
SELECT 'diet.plan', provider_key, model, 8000, NULL, '孩子饮食：说一句/拍食谱改计划', NOW()
  FROM pa_llm_route WHERE task = 'memory.extract';
