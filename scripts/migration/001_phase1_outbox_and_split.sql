-- ============================================================
-- Outbox 基础设施表与非 Group Schema 统一 DDL
-- 执行前请先做全量 DB 备份！
-- ============================================================

-- ==================== 1. Outbox 事件表 ====================
CREATE TABLE IF NOT EXISTS `outbox_events` (
    `id`            BIGINT        NOT NULL AUTO_INCREMENT,
    `event_type`    VARCHAR(128)  NOT NULL COMMENT '领域事件类型: user_created / profile_display_changed / account.deleted',
    `entity_id`     VARCHAR(64)   NOT NULL COMMENT '事件分区键（通常是 user_uuid）',
    `payload`       LONGTEXT      NOT NULL COMMENT '事件负载 JSON',
    `created_at`    DATETIME(3)   NOT NULL COMMENT '创建时间',
    PRIMARY KEY (`id`),
    INDEX `idx_outbox_event_type_created` (`event_type`, `created_at`),
    INDEX `idx_outbox_entity_id` (`entity_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='Outbox 事件表';

-- ==================== 2. 幂等消费记录表 ====================
CREATE TABLE IF NOT EXISTS `idempotent_events` (
    `id`           BIGINT      NOT NULL AUTO_INCREMENT,
    `event_type`   VARCHAR(64) NOT NULL COMMENT '事件类型',
    `event_id`     VARCHAR(64) NOT NULL COMMENT '事件唯一 ID',
    `processed_at` DATETIME(3) NOT NULL COMMENT '处理时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_type_event` (`event_type`, `event_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='幂等消费记录表';

-- ==================== 3. user_account 表（auth-service 归属）====================
CREATE TABLE IF NOT EXISTS `user_account` (
    `id`              BIGINT       NOT NULL AUTO_INCREMENT,
    `user_uuid`       CHAR(20)     NOT NULL COMMENT '用户唯一 ID',
    `email`           VARCHAR(100) NOT NULL COMMENT '邮箱（登录凭证）',
    `telephone`       VARCHAR(20)  NULL     DEFAULT NULL COMMENT '手机号',
    `password_hash`   CHAR(60)     NOT NULL COMMENT 'bcrypt 密码哈希',
    `status`          TINYINT      NOT NULL DEFAULT 0 COMMENT '状态: 0=正常 1=注销',
    `is_admin`        TINYINT      NOT NULL DEFAULT 0 COMMENT '是否管理员',
    `login_nickname`  VARCHAR(20)  NOT NULL DEFAULT '' COMMENT '登录展示用昵称（冗余字段，权威在 user_profile）',
    `login_avatar`    VARCHAR(255) NOT NULL DEFAULT '' COMMENT '登录展示用头像（冗余字段，权威在 user_profile）',
    `last_login_at`   DATETIME(3)  NULL     COMMENT '最近登录时间',
    `created_at`      DATETIME(3)  NOT NULL COMMENT '创建时间',
    `updated_at`      DATETIME(3)  NOT NULL COMMENT '更新时间',
    `deleted_at`      DATETIME(3)  NULL     COMMENT '注销时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user_uuid` (`user_uuid`),
    UNIQUE KEY `uk_email` (`email`),
    UNIQUE KEY `uk_telephone` (`telephone`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='用户账号表（auth-service 归属）';

-- ==================== 4. user_profile 表（user-service 归属）====================
CREATE TABLE IF NOT EXISTS `user_profile` (
    `id`           BIGINT       NOT NULL AUTO_INCREMENT,
    `user_uuid`    CHAR(20)     NOT NULL COMMENT '用户唯一 ID',
    `nickname`     VARCHAR(20)  NOT NULL DEFAULT '' COMMENT '昵称',
    `avatar`       VARCHAR(255) NOT NULL DEFAULT '' COMMENT '头像 URL',
    `gender`       TINYINT      NOT NULL DEFAULT 3  COMMENT '性别: 1=男 2=女 3=未知',
    `birthday`     DATE         NULL     DEFAULT NULL COMMENT '生日',
    `signature`    VARCHAR(100) NOT NULL DEFAULT '' COMMENT '个性签名',
    `qrcode_token` VARCHAR(64)  NULL     DEFAULT NULL COMMENT '二维码 token',
    `created_at`   DATETIME(3)  NOT NULL COMMENT '创建时间',
    `updated_at`   DATETIME(3)  NOT NULL COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user_uuid` (`user_uuid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='用户资料表（user-service 归属）';

-- ==================== 5. 表名标准化（复数命名）====================
-- 注意：RENAME TABLE 是原子操作，但会短暂锁表，建议在低峰期执行

RENAME TABLE `user_relation` TO `user_relations`;
RENAME TABLE `apply_request` TO `apply_requests`;
RENAME TABLE `device_session` TO `device_sessions`;
RENAME TABLE `group_info` TO `groups`;
RENAME TABLE `group_member` TO `group_members`;

-- ==================== 6. group_join_requests 表（group-service 独占）====================
CREATE TABLE IF NOT EXISTS `group_join_requests` (
    `id`             BIGINT       NOT NULL AUTO_INCREMENT,
    `group_uuid`     CHAR(20)     NOT NULL COMMENT '群 uuid',
    `applicant_uuid` CHAR(20)     NOT NULL COMMENT '申请人 uuid',
    `status`         TINYINT      NOT NULL DEFAULT 0 COMMENT '状态: 0=待处理 1=通过 2=拒绝',
    `reason`         VARCHAR(255) NULL     DEFAULT NULL COMMENT '申请附言',
    `reviewer_uuid`  CHAR(20)     NULL     DEFAULT NULL COMMENT '处理人 uuid',
    `review_remark`  VARCHAR(255) NULL     DEFAULT NULL COMMENT '处理备注',
    `reviewed_at`    DATETIME(3)  NULL     DEFAULT NULL COMMENT '处理时间',
    `created_at`     DATETIME(3)  NOT NULL COMMENT '创建时间',
    `updated_at`     DATETIME(3)  NOT NULL COMMENT '更新时间',
    `deleted_at`     DATETIME(3)  NULL     DEFAULT NULL COMMENT '删除时间',
    PRIMARY KEY (`id`),
    KEY `idx_group_join_pending` (`group_uuid`, `status`, `deleted_at`, `created_at`, `id`),
    KEY `idx_group_join_applicant` (`applicant_uuid`, `status`, `deleted_at`, `created_at`, `id`),
    KEY `idx_group_join_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='群入群申请表（group-service 归属）';
