-- 005_group_cache_projection_version.sql
-- 为 group.cache v2 投影契约增加群聚合版本。
--
-- 新写事务会在每条 outbox 事件写入前递增 cache_version；已有群没有历史事件版本，
-- 因此迁移时统一从 1 起步，随后由对账任务按该版本重建 Redis。旧事件缺少
-- schema_version / projection_version，会被严格消费者直接送入 dead_events，不参与投影。
USE `chat_server`;

ALTER TABLE `groups`
  ADD COLUMN `cache_version` BIGINT NOT NULL DEFAULT 0 COMMENT '群缓存投影严格递增版本'
  AFTER `status`;

UPDATE `groups`
SET `cache_version` = 1
WHERE `cache_version` = 0;
