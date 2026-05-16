-- ============================================================
-- Group P0/P1 扩展：全员禁言与申请列表查询索引
--
-- 说明：
-- 1. groups.mute_all 承载群级全员禁言开关，发送权限检查会与成员角色、单人禁言组合判断；
-- 2. group_join_requests 新增复合索引，支撑“我的申请筛选”和“审批历史筛选”分页查询；
-- 3. 本项目当前不做向后兼容迁移，执行前请先备份数据库。
-- ============================================================

ALTER TABLE `groups`
  ADD COLUMN `mute_all` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否全员禁言,0否 1是' AFTER `add_mode`;

ALTER TABLE `group_join_requests`
  ADD KEY `idx_group_join_group_applicant_id` (`group_uuid`, `applicant_uuid`, `deleted_at`, `id`),
  ADD KEY `idx_group_join_reviewed` (`group_uuid`, `status`, `deleted_at`, `reviewed_at`, `id`);
