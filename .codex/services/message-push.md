# Message-push 服务

## 角色

- 消费 Kafka 的 msg.push 与 realtime.push。
- 从 Redis 读取在线路由，群目标通过 Group 展开，再调用 Connect gRPC 完成在线下发。
- 不拥有业务表，也不决定好友、群成员或消息状态等领域事实。

## 修改前优先阅读

- apps/message-push/cmd/providers.go
- apps/message-push/internal/consumer/consumer.go
- apps/message-push/internal/consumer/realtime_handler.go
- apps/message-push/internal/route/repository.go
- apps/message-push/internal/connectcli
- proto/msg/msg_push_event.proto
- proto/realtime/realtime_event.proto

## 下行行为

- 单聊 MSG_PUSH/MSG_RECALL 推给接收方路由和发送方的其他设备。
- 群 MSG_PUSH/MSG_RECALL 展开除发送者外的成员，并同步发送方其他设备。
- MSG_MARK_READ 同步当前用户其他设备；MSG_READ_RECEIPT 只走接收方路由。
- realtime.push 支持用户、指定设备、用户列表、群成员和群管理员等目标类型。
- 路由会按 user_uuid 加 device_id 去重，过期活动时间的路由会被过滤。
- 普通消息的 ACK 仅针对 seq 大于 0 的 MSG_PUSH；业务事件或 seq=0 事件不能假定具备同样补洞能力。

## 重试与可靠性边界

- 本服务使用自定义消费循环，不使用 pkg/kafka.NewManualCommitConsumer。
- 本地重试有限，默认 3 次并带短退避和单次处理预算。
- 至少一个目标设备推送成功即可视为该事件已处理；全部失败或预算耗尽后会记录告警/指标并让出 offset。
- 不写 dead_events。它优先避免单条消息阻塞分区，代价是实时下行不承诺至少一次。
- MSG_PUSH 的客户端可按 seq 拉取补洞；撤回、已读和 realtime.push 必须有独立查询/刷新兜底。

## 修改陷阱

- 改群扩散时同时检查 Group 成员查询、Msg 的群成员资格投影和在线路由；它们不是同一个数据源。
- 推送成功不等于客户端持久化成功；ACK 和最终消息状态仍在 Connect/Msg/客户端协作中完成。
- 不要把实时提醒改成业务事实来源；Relation/Group 的 realtime.push 是尽力副作用。
