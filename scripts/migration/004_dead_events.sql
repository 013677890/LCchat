-- 004_dead_events.sql
-- 新增消费死信表：手动提交消费者(user/relation/group/auth)在「有界重试」预算耗尽后，
-- 把毒/持久失败消息旁路到此表并提交 offset，解除队头阻塞(head-of-line blocking)。
-- 消息不丢、可查询、可重放(重放安全由 idempotent_events 保证)。
USE `chat_server`;
CREATE TABLE IF NOT EXISTS `dead_events` (
  `id` BIGINT NOT NULL AUTO_INCREMENT COMMENT '自增主键',
  `source` VARCHAR(128) NOT NULL COMMENT '来源消费者标识(service:topic)',
  `topic` VARCHAR(191) NOT NULL COMMENT 'Kafka topic',
  `kafka_partition` INT NOT NULL DEFAULT 0 COMMENT 'Kafka 分区',
  `kafka_offset` BIGINT NOT NULL DEFAULT 0 COMMENT 'Kafka offset',
  `msg_key` VARCHAR(191) NOT NULL DEFAULT '' COMMENT 'Kafka 消息 key',
  `event_type` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '事件类型(尽力解析)',
  `payload` LONGBLOB NOT NULL COMMENT '原始消息字节',
  `error_msg` TEXT COMMENT '最后一次失败错误',
  `attempts` INT NOT NULL DEFAULT 0 COMMENT '原地重试次数',
  `status` VARCHAR(16) NOT NULL DEFAULT 'pending' COMMENT 'pending|replayed|discarded',
  `first_failed_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '首次失败时间',
  `last_failed_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '最后失败时间',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
  PRIMARY KEY (`id`),
  KEY `idx_dead_source_status` (`source`, `status`),
  KEY `idx_dead_status_created` (`status`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='消费死信(队头阻塞旁路)';
