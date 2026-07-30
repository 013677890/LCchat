package repository

import (
	"encoding/json"
)

// friendMeta 是好友 Hash 缓存里每个 field 对应的紧凑元数据结构。
//
// relation-service 的好友缓存不直接冗余 profile 信息，只保留关系域自身需要的 remark、
// group_tag、source 和 updated_at，这样既能支持高频关系判断，也能避免缓存结构膨胀。
type friendMeta struct {
	Remark    string `json:"remark"`
	GroupTag  string `json:"group_tag"`
	Source    string `json:"source"`
	UpdatedAt int64  `json:"updated_at"`
}

// buildFriendMetaJSON 将好友元数据编码为 Redis Hash value。
//
// 这里在编码失败时返回空对象而不是直接报错，是因为该函数主要用于缓存路径，缓存写失败
// 不应阻塞主业务；调用方仍会保留 DB 作为最终权威来源。
func buildFriendMetaJSON(remark, groupTag, source string, updatedAt int64) string {
	meta := friendMeta{
		Remark:    remark,
		GroupTag:  groupTag,
		Source:    source,
		UpdatedAt: updatedAt,
	}
	data, err := json.Marshal(&meta)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// parseFriendMetaJSON 解析好友元数据缓存。
//
// 解析失败时由调用方决定是否忽略缓存并回源 DB，因此这里直接把错误向上返回，便于上层
// 采取“继续降级但不中断主流程”的策略。
func parseFriendMetaJSON(raw string) (*friendMeta, error) {
	var meta friendMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
