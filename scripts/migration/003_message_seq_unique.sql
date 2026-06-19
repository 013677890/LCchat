-- ============================================================
-- Message seq 唯一约束修复
--
-- 说明：
-- 1. 执行前必须先备份数据库；
-- 2. 若下方查重 SQL 返回数据，需要先人工清理重复 conv_id + seq，否则唯一索引会创建失败；
-- 3. 本项目当前不做向后兼容迁移，直接将普通索引升级为唯一索引。
-- ============================================================

SELECT conv_id, seq, COUNT(*) AS cnt
FROM `message`
GROUP BY conv_id, seq
HAVING cnt > 1;

ALTER TABLE `message`
  DROP INDEX `idx_conv_seq`,
  ADD UNIQUE KEY `uidx_conv_seq` (`conv_id`, `seq`);
