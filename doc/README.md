# LCchat 文档索引

## 目录结构

```
doc/
├── api/            API 接口规范
├── architecture/   架构图（SVG、XML）
├── design/         技术设计文档（消息链路、数据库、限流、Redis 等）
├── dev-notes/      开发笔记（问题记录、优化、风险记账、错误审查）
├── guides/         使用说明（MinIO、验证码、监控）
├── message_doc/    消息服务详细文档
├── user_doc/       用户服务详细文档
└── graph/          Mermaid 图表
```

## 快速导航

### 架构
- [架构图](architecture/架构图.svg)
- [请求认证流程](architecture/请求认证.svg)

### 设计文档
- [消息下行链路设计（Connect Kafka Consumer）](design/Connect消息下行链路设计.md)
- [消息链路设计（JSON上行 / Proto下行）](design/消息链路设计(JSON上行_Proto下行).md)
- [数据库设计](design/数据库设计.md)
- [Redis Key 设计](design/redis_key_design.md)
- [限流设计](design/rate_limit限流.md)
- [Redis 重试机制](design/redis重试文档.md)

### API
- [API 接口规范](api/API接口规范.md)

### 服务文档
- [消息服务文档](message_doc/)
- [用户服务文档](user_doc/)

### 开发笔记
- [代码错误审查报告](dev-notes/代码错误审查报告.md)
- [行为风险记账](dev-notes/实现行为风险记账.md)
- [未来优化计划](dev-notes/未来优化.md)
- [日志与错误迁移](dev-notes/日志与错误迁移.md)

### 使用说明
- [MinIO 使用方法](guides/minio使用方法.md)
- [验证码邮件发送](guides/验证码邮件发送使用说明.md)
- [监控集成说明](guides/监控集成说明.md)
- [实际测试指南](guides/实际测试指南.md)
- [k3s 迁移方案](guides/k3s迁移方案.md)
