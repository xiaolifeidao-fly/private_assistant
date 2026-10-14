-- M3 补齐：分身可查看自己的完整档案和人设卡。
-- 新库由 service/bot 的默认值直接带上 get_person；这里负责升级已有库，
-- 只追加缺失的工具，不覆盖管理端的其他配置。
UPDATE pa_bot
	 SET tools = JSON_ARRAY_APPEND(COALESCE(tools, JSON_ARRAY()), '$', 'get_person'),
       updated_at = NOW()
 WHERE kind = 'persona'
   AND (tools IS NULL OR JSON_CONTAINS(tools, JSON_QUOTE('get_person')) = 0);
