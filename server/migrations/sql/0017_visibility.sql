-- 记忆、原始记录、提醒按账号区分可见范围（common/viewer）：
--   owner_account  归属账号（App 账号的 account_key），空串表示没有归属
--   visibility     private 只有归属账号自己看得到 / family 全家可见
-- 新记录默认 private，我自己的账号可以改成全家可见，奶奶的账号只能 private。
-- 已有的数据都是我记的：归到家里第一个手机端管理员账号名下，设为 private。没有这样的账号就保持全家可见。

ALTER TABLE pa_memory
  ADD COLUMN owner_account VARCHAR(40) NOT NULL DEFAULT '' COMMENT '归属账号' AFTER created_by,
  ADD COLUMN visibility VARCHAR(10) NOT NULL DEFAULT 'family' COMMENT 'private 仅自己 / family 全家' AFTER owner_account,
  ADD KEY idx_memory_owner (family_key, owner_account, visibility);

ALTER TABLE pa_source_note
  ADD COLUMN owner_account VARCHAR(40) NOT NULL DEFAULT '' COMMENT '归属账号' AFTER created_by,
  ADD COLUMN visibility VARCHAR(10) NOT NULL DEFAULT 'family' COMMENT 'private 仅自己 / family 全家' AFTER owner_account;

ALTER TABLE pa_reminder
  ADD COLUMN owner_account VARCHAR(40) NOT NULL DEFAULT '' COMMENT '归属账号，推送只发给能看到它的账号' AFTER source_key,
  ADD COLUMN visibility VARCHAR(10) NOT NULL DEFAULT 'family' COMMENT 'private 仅自己 / family 全家' AFTER owner_account;

UPDATE pa_memory t SET owner_account = COALESCE((SELECT a.account_key FROM pa_account a
  WHERE a.realm = 'app' AND a.role = 'admin' AND a.status = 'active' AND a.family_key = t.family_key ORDER BY a.id LIMIT 1), '');
UPDATE pa_memory SET visibility = 'private' WHERE owner_account <> '';

UPDATE pa_source_note t SET owner_account = COALESCE((SELECT a.account_key FROM pa_account a
  WHERE a.realm = 'app' AND a.role = 'admin' AND a.status = 'active' AND a.family_key = t.family_key ORDER BY a.id LIMIT 1), '');
UPDATE pa_source_note SET visibility = 'private' WHERE owner_account <> '';

UPDATE pa_reminder t SET owner_account = COALESCE((SELECT a.account_key FROM pa_account a
  WHERE a.realm = 'app' AND a.role = 'admin' AND a.status = 'active' AND a.family_key = t.family_key ORDER BY a.id LIMIT 1), '');
UPDATE pa_reminder SET visibility = 'private' WHERE owner_account <> '';
