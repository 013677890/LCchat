SET NAMES utf8mb4;
SET time_zone = '+00:00';
SET FOREIGN_KEY_CHECKS = 0;

CREATE DATABASE IF NOT EXISTS `chat_server`
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;
USE `chat_server`;

CREATE TABLE IF NOT EXISTS `user_info` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增id',
  `uuid` CHAR(20) NOT NULL COMMENT '用户唯一id',
  `nickname` VARCHAR(20) NOT NULL COMMENT '昵称',
  `telephone` VARCHAR(20) DEFAULT NULL COMMENT '电话',
  `email` VARCHAR(100) DEFAULT NULL COMMENT '邮箱',
  `avatar` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '头像',
  `gender` TINYINT DEFAULT 3 COMMENT '性别,1.男 2.女 3.未知',
  `signature` VARCHAR(100) DEFAULT '' COMMENT '个性签名',
  `password` CHAR(60) NOT NULL COMMENT '密码哈希',
  `birthday` DATE DEFAULT NULL COMMENT '生日',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '删除时间',
  `is_admin` TINYINT NOT NULL DEFAULT 0 COMMENT '是否是管理员,0.不是 1.是',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '状态,0.正常 1.禁用',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_info_uuid` (`uuid`),
  UNIQUE KEY `uk_user_info_telephone` (`telephone`),
  UNIQUE KEY `uk_user_info_email` (`email`),
  KEY `idx_user_info_created_at` (`created_at`),
  KEY `idx_user_info_deleted_at` (`deleted_at`),
  KEY `idx_user_info_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户基础信息';

CREATE TABLE IF NOT EXISTS `group_info` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增id',
  `uuid` CHAR(20) NOT NULL COMMENT '群组唯一id',
  `name` VARCHAR(64) NOT NULL COMMENT '群名称',
  `notice` VARCHAR(500) DEFAULT NULL COMMENT '群公告',
  `member_cnt` INT NOT NULL DEFAULT 1 COMMENT '群人数',
  `owner_uuid` CHAR(20) NOT NULL COMMENT '群主uuid',
  `add_mode` TINYINT NOT NULL DEFAULT 0 COMMENT '加群方式,0.直接 1.审核',
  `avatar` VARCHAR(255) NOT NULL DEFAULT 'https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png' COMMENT '群头像URL',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '状态,0.正常 1.禁用 2.解散',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_group_info_uuid` (`uuid`),
  KEY `idx_group_info_owner_uuid` (`owner_uuid`),
  KEY `idx_group_info_status` (`status`),
  KEY `idx_group_info_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='群基础信息';

CREATE TABLE IF NOT EXISTS `group_member` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增id',
  `group_uuid` CHAR(20) NOT NULL COMMENT '群uuid',
  `user_uuid` CHAR(20) NOT NULL COMMENT '用户uuid',
  `role` TINYINT NOT NULL DEFAULT 0 COMMENT '0成员 1管理员 2群主',
  `remark` VARCHAR(64) DEFAULT NULL COMMENT '群名片/备注',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '0正常 1退出 2踢出 3待审核',
  `mute_until` DATETIME(3) DEFAULT NULL COMMENT '禁言到期时间',
  `inviter_uuid` CHAR(20) DEFAULT NULL COMMENT '邀请人uuid',
  `joined_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '入群时间',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uidx_group_user` (`group_uuid`, `user_uuid`),
  KEY `idx_group_member_group_uuid` (`group_uuid`),
  KEY `idx_group_member_user_uuid` (`user_uuid`),
  KEY `idx_group_member_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='群成员关系';

CREATE TABLE IF NOT EXISTS `user_relation` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增id',
  `user_uuid` CHAR(20) NOT NULL COMMENT '用户uuid',
  `peer_uuid` CHAR(20) NOT NULL COMMENT '对端用户uuid',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '关系状态 0.正常 1.拉黑(原先为好友) 2.删除 3.拉黑(原先非好友)',
  `remark` VARCHAR(64) DEFAULT NULL COMMENT '好友备注',
  `source` VARCHAR(64) DEFAULT NULL COMMENT '添加来源',
  `group_tag` VARCHAR(32) DEFAULT NULL COMMENT '标签',
  `blacklisted_at` DATETIME(3) DEFAULT NULL COMMENT '拉黑时间',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uidx_user_peer` (`user_uuid`, `peer_uuid`),
  KEY `idx_user_updated_at` (`user_uuid`, `updated_at`),
  KEY `idx_peer_uuid` (`peer_uuid`),
  KEY `idx_user_status_deleted_created` (`user_uuid`, `status`, `deleted_at`, `created_at`, `id`),
  KEY `idx_user_blacklist_deleted_time` (`user_uuid`, `status`, `deleted_at`, `blacklisted_at`, `id`),
  KEY `idx_user_relation_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户单向关系';

CREATE TABLE IF NOT EXISTS `apply_request` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '自增id',
  `apply_type` TINYINT NOT NULL COMMENT '0好友 1加群',
  `applicant_uuid` CHAR(20) NOT NULL COMMENT '申请人uuid',
  `target_uuid` CHAR(20) NOT NULL COMMENT '目标uuid',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '0待处理 1通过 2拒绝 3过期',
  `is_read` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '申请是否已读',
  `reason` VARCHAR(255) DEFAULT NULL COMMENT '申请附言',
  `source` VARCHAR(32) DEFAULT NULL COMMENT '申请来源',
  `handle_user_uuid` CHAR(20) DEFAULT NULL COMMENT '处理人uuid',
  `handle_remark` VARCHAR(255) DEFAULT NULL COMMENT '处理备注',
  `expired_at` DATETIME(3) DEFAULT NULL COMMENT '过期时间',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_applicant_target` (`applicant_uuid`, `target_uuid`),
  KEY `idx_apply_pending_list` (`apply_type`, `target_uuid`, `status`, `deleted_at`, `created_at`, `id`),
  KEY `idx_apply_sent_list` (`apply_type`, `applicant_uuid`, `status`, `deleted_at`, `created_at`, `id`),
  KEY `idx_apply_target_read` (`target_uuid`, `apply_type`, `is_read`, `deleted_at`),
  KEY `idx_apply_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='好友/加群申请';

CREATE TABLE IF NOT EXISTS `device_session` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_uuid` CHAR(20) NOT NULL COMMENT '用户uuid',
  `device_id` VARCHAR(64) NOT NULL COMMENT '设备唯一指纹',
  `device_name` VARCHAR(64) NOT NULL DEFAULT 'Unknown Device' COMMENT '设备名称',
  `platform` VARCHAR(32) NOT NULL COMMENT '平台',
  `app_version` VARCHAR(32) DEFAULT NULL COMMENT 'APP版本',
  `ip` VARCHAR(64) DEFAULT NULL COMMENT '登录IP',
  `user_agent` VARCHAR(512) DEFAULT NULL COMMENT 'User Agent',
  `expire_at` DATETIME(3) DEFAULT NULL COMMENT '过期时间',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '状态',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uidx_user_device` (`user_uuid`, `device_id`),
  KEY `idx_device_expire_at` (`expire_at`),
  KEY `idx_device_deleted_at` (`deleted_at`),
  KEY `idx_device_user_updated` (`user_uuid`, `updated_at`, `id`),
  KEY `idx_device_user_status_deleted` (`user_uuid`, `status`, `deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='设备会话';

CREATE TABLE IF NOT EXISTS `conversation` (
  `id` BIGINT NOT NULL AUTO_INCREMENT COMMENT '自增id',
  `conv_id` VARCHAR(128) NOT NULL COMMENT '会话ID',
  `type` TINYINT NOT NULL COMMENT '1单聊 2群聊',
  `owner_uuid` CHAR(20) NOT NULL COMMENT '会话归属用户uuid',
  `target_uuid` CHAR(20) NOT NULL COMMENT '单聊为对端uuid，群聊为群uuid',
  `last_msg_id` CHAR(64) DEFAULT NULL COMMENT '最后消息ID',
  `last_msg_at` DATETIME(3) DEFAULT NULL COMMENT '最后消息时间',
  `last_msg_preview` VARCHAR(255) DEFAULT NULL COMMENT '最后消息预览JSON',
  `max_seq` BIGINT NOT NULL DEFAULT 0 COMMENT '会话内当前最大seq',
  `read_seq` BIGINT NOT NULL DEFAULT 0 COMMENT '已读最大seq',
  `clear_seq` BIGINT NOT NULL DEFAULT 0 COMMENT '会话清空位点',
  `unread_count` BIGINT NOT NULL DEFAULT 0 COMMENT '未读数',
  `mute` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '免打扰',
  `pin` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '置顶',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '0正常 1关闭/删除',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uidx_owner_conv` (`owner_uuid`, `target_uuid`),
  KEY `idx_owner_status_update` (`owner_uuid`, `status`, `updated_at`),
  KEY `idx_conversation_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会话元数据';

CREATE TABLE IF NOT EXISTS `message` (
  `id` BIGINT NOT NULL AUTO_INCREMENT COMMENT '自增id',
  `conv_id` VARCHAR(128) NOT NULL COMMENT '会话ID',
  `seq` BIGINT NOT NULL COMMENT '会话内序号',
  `msg_id` CHAR(64) NOT NULL COMMENT '全局消息ID',
  `client_msg_id` CHAR(64) NOT NULL COMMENT '客户端幂等ID',
  `from_uuid` CHAR(20) NOT NULL COMMENT '发送者uuid',
  `device_id` CHAR(64) NOT NULL COMMENT '发送设备ID',
  `msg_type` SMALLINT NOT NULL COMMENT '消息类型',
  `content` JSON NOT NULL COMMENT '消息内容JSON',
  `status` TINYINT NOT NULL DEFAULT 0 COMMENT '0正常 1撤回 2删除',
  `reply_to_msg_id` CHAR(64) NOT NULL DEFAULT '' COMMENT '引用消息ID',
  `at_users` JSON DEFAULT NULL COMMENT '被@用户UUID列表',
  `send_time` DATETIME(3) DEFAULT NULL COMMENT '发送时间',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_message_msg_id` (`msg_id`),
  UNIQUE KEY `uidx_sender_client` (`client_msg_id`, `from_uuid`, `device_id`),
  KEY `idx_conv_seq` (`conv_id`, `seq`),
  KEY `idx_conv_time` (`conv_id`, `send_time`),
  KEY `idx_message_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='聊天消息';

CREATE TABLE IF NOT EXISTS `group_conversation` (
  `group_uuid` CHAR(20) NOT NULL COMMENT '群组唯一id(业务主键，一对一关系无需自增ID)',
  `max_seq` BIGINT NOT NULL DEFAULT 0 COMMENT '群消息当前最大seq',
  `last_msg_id` CHAR(64) DEFAULT NULL COMMENT '最后一条消息的ID(ULID)',
  `last_msg_preview` VARCHAR(255) DEFAULT NULL COMMENT '最后消息预览',
  `last_msg_at` DATETIME(3) DEFAULT NULL COMMENT '最后消息时间(做活跃度排序)',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '状态最后变更时间',
  PRIMARY KEY (`group_uuid`),
  KEY `idx_last_msg_at` (`last_msg_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='群会话状态(热数据,每条群消息更新一次)';

SET FOREIGN_KEY_CHECKS = 1;
