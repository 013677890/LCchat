package usecase

import "strings"

// extractPeerUuid 从 P2P 单聊 conv_id 中提取对端 UUID
//
// conv_id 格式: "p2p-{uuid_a}-{uuid_b}" （字典序排列）
// 给定其中一方的 UUID，返回另一方的 UUID
//
// 示例:
//
//	extractPeerUuid("p2p-alice-bob", "alice") → "bob"
//	extractPeerUuid("p2p-alice-bob", "bob")   → "alice"
func extractPeerUuid(convId string, selfUuid string) string {
	// 去掉 "p2p-" 前缀
	body := strings.TrimPrefix(convId, "p2p-")

	// UUID 固定 20 字符，按 "p2p-{20}-{20}" 格式切割
	// 但为了安全兼容，按第一个 selfUuid 出现位置来判断
	parts := strings.SplitN(body, "-", 2)
	if len(parts) != 2 {
		return "" // conv_id 格式异常
	}

	if parts[0] == selfUuid {
		return parts[1]
	}
	return parts[0]
}
