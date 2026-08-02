package authcli

import (
	"context"
	"fmt"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"google.golang.org/grpc"
)

// Client 封装 relation → auth 的内部账号查询。
//
// relation 域不拥有任何账号事实，好友申请等写路径需要的"目标账号是否真实存在"
// 边界判断通过 InternalAuthService.BatchCheckAccountStatus 查询 auth-service 获得，
// 避免向不存在或已注销的账号落库脏申请。
type Client struct {
	internalAuthClient authpb.InternalAuthServiceClient
}

// NewClient 创建 auth 内部账号查询客户端。
func NewClient(conn *grpc.ClientConn) *Client {
	if conn == nil {
		return nil
	}
	return &Client{internalAuthClient: authpb.NewInternalAuthServiceClient(conn)}
}

// IsAccountVisible 判断目标账号是否存在且未注销。
//
// 语义与 auth 侧 proto 保持一致：exists=false 或 status=1（已注销）都视为不可见；
// 查询失败时向上返回错误，由调用方按 fail-close 处理，禁止降级放行。
func (c *Client) IsAccountVisible(ctx context.Context, userUUID string) (bool, error) {
	if userUUID == "" {
		return false, apperr.New(consts.CodeParamError)
	}
	if c == nil || c.internalAuthClient == nil {
		return false, apperr.NewWithMessage(consts.CodeInternalError, "账号校验客户端未初始化")
	}

	resp, err := c.internalAuthClient.BatchCheckAccountStatus(ctx, &authpb.BatchCheckAccountStatusRequest{
		UserUuids: []string{userUUID},
	})
	if err != nil {
		return false, normalizeRemoteError(err, "查询账号状态失败")
	}

	for _, item := range resp.GetItems() {
		if item == nil || item.GetUserUuid() != userUUID {
			continue
		}
		return item.GetExists() && item.GetStatus() == 0, nil
	}

	// auth 未返回该账号对应条目时按不存在处理，保持与 exists=false 相同的保守语义。
	return false, nil
}

// normalizeRemoteError 与 msg/usercli 保持一致的跨服务错误归一化策略：
// 下游业务错误原样透传，传输层错误统一包装为内部错误。
func normalizeRemoteError(err error, fallbackMsg string) error {
	if err == nil {
		return nil
	}
	appErr := apperr.FromStatus(err)
	if code := apperr.Code(appErr); code != consts.CodeInternalError {
		return appErr
	}
	return apperr.Wrap(fmt.Errorf("%s: %w", fallbackMsg, appErr), consts.CodeInternalError, fallbackMsg)
}
