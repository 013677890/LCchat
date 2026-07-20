package service

import (
	"context"

	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/pb"
	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
)

const simpleUserBatchSize = 100

// batchGetSimpleUserInfo 批量读取关系列表展示所需的用户摘要。
//
// 好友与黑名单列表采用完全相同的补全规则，因此在 service 包内共享这一段数据整理逻辑：
//  1. 忽略空 UUID，并按首次出现顺序去重；
//  2. 每批最多查询 100 个用户，避免单次 gRPC 请求过大；
//  3. 忽略下游返回的空用户或空 UUID；
//  4. 某批失败时保留此前批次的结果并返回错误，由各业务调用方按原有策略决定是否降级。
//
// 该函数只合并数据获取机制，不负责日志或降级响应，避免把好友与黑名单的业务语义耦合在一起。
func batchGetSimpleUserInfo(ctx context.Context, userClient pb.UserServiceClient, uuids []string) (map[string]*dto.SimpleUserInfo, error) {
	result := make(map[string]*dto.SimpleUserInfo)
	if len(uuids) == 0 {
		return result, nil
	}

	unique := make([]string, 0, len(uuids))
	seen := make(map[string]struct{}, len(uuids))
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		if _, exists := seen[uuid]; exists {
			continue
		}
		seen[uuid] = struct{}{}
		unique = append(unique, uuid)
	}

	for start := 0; start < len(unique); start += simpleUserBatchSize {
		end := start + simpleUserBatchSize
		if end > len(unique) {
			end = len(unique)
		}

		grpcResp, err := userClient.BatchGetProfile(ctx, &userpb.BatchGetProfileRequest{
			UserUuids: unique[start:end],
		})
		if err != nil {
			return result, err
		}

		for _, user := range grpcResp.Users {
			if user == nil || user.Uuid == "" {
				continue
			}
			result[user.Uuid] = dto.ConvertSimpleUserInfoFromProto(user)
		}
	}

	return result, nil
}
