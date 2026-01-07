package model


//枚举消息类型
// 0-99: 普通聊天消息 (有气泡)
const (
    MsgTypeText   = 1
    MsgTypeImage  = 2
    MsgTypeAudio  = 3
    MsgTypeVideo  = 4
    MsgTypeFile   = 5
)

// 100-199: 系统控制消息 (无气泡，居中灰条)
const (
    MsgTypeRevoke    = 100 // 撤回
    MsgTypeGroupJoin = 101 // 加入群聊
    MsgTypeGroupExit = 102 // 退出/被踢
    MsgTypeMute      = 103 // 禁言通知
)