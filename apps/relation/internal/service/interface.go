package service
import (
	"context"
	pb "github.com/013677890/LCchat-Backend/apps/relation/pb"
)
// ==================== 好友服务接口 ====================
// IFriendService 好友服务接口
// 职责：好友申请、好友列表、备注标签
type IFriendService interface {
	// SendFriendApply 发送好友申请
	SendFriendApply(ctx context.Context, req *pb.SendFriendApplyRequest) (*pb.SendFriendApplyResponse, error)
	// GetFriendApplyList 获取好友申请列表
	GetFriendApplyList(ctx context.Context, req *pb.GetFriendApplyListRequest) (*pb.GetFriendApplyListResponse, error)
	// GetSentApplyList 获取发出的申请列表
	GetSentApplyList(ctx context.Context, req *pb.GetSentApplyListRequest) (*pb.GetSentApplyListResponse, error)
	// HandleFriendApply 处理好友申请
	HandleFriendApply(ctx context.Context, req *pb.HandleFriendApplyRequest) error
	// GetUnreadApplyCount 获取未读申请数量
	GetUnreadApplyCount(ctx context.Context, req *pb.GetUnreadApplyCountRequest) (*pb.GetUnreadApplyCountResponse, error)
	// MarkApplyAsRead 标记申请已读
	MarkApplyAsRead(ctx context.Context, req *pb.MarkApplyAsReadRequest) error
	// GetFriendList 获取好友列表
	GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error)
	// SyncFriendList 好友增量同步
	SyncFriendList(ctx context.Context, req *pb.SyncFriendListRequest) (*pb.SyncFriendListResponse, error)
	// DeleteFriend 删除好友
	DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) error
	// SetFriendRemark 设置好友备注
	SetFriendRemark(ctx context.Context, req *pb.SetFriendRemarkRequest) error
	// SetFriendTag 设置好友标签
	SetFriendTag(ctx context.Context, req *pb.SetFriendTagRequest) error
	// GetTagList 获取标签列表
	GetTagList(ctx context.Context, req *pb.GetTagListRequest) (*pb.GetTagListResponse, error)
	// CheckIsFriend 判断是否好友
	CheckIsFriend(ctx context.Context, req *pb.CheckIsFriendRequest) (*pb.CheckIsFriendResponse, error)
	// BatchCheckIsFriend 批量判断是否好友
	BatchCheckIsFriend(ctx context.Context, req *pb.BatchCheckIsFriendRequest) (*pb.BatchCheckIsFriendResponse, error)
	// GetRelationStatus 获取关系状态
	GetRelationStatus(ctx context.Context, req *pb.GetRelationStatusRequest) (*pb.GetRelationStatusResponse, error)
}
// ==================== 黑名单服务接口 ====================
// IBlacklistService 黑名单服务接口
// 职责：拉黑、取消拉黑、黑名单列表、判断是否拉黑
type IBlacklistService interface {
	// AddBlacklist 拉黑用户
	AddBlacklist(ctx context.Context, req *pb.AddBlacklistRequest) error
	// RemoveBlacklist 取消拉黑
	RemoveBlacklist(ctx context.Context, req *pb.RemoveBlacklistRequest) error
	// GetBlacklistList 获取黑名单列表
	GetBlacklistList(ctx context.Context, req *pb.GetBlacklistListRequest) (*pb.GetBlacklistListResponse, error)
	// CheckIsBlacklist 判断是否拉黑
	CheckIsBlacklist(ctx context.Context, req *pb.CheckIsBlacklistRequest) (*pb.CheckIsBlacklistResponse, error)
}
// ==================== 别名类型定义 ====================
// FriendService 别名 IFriendService
type FriendService = IFriendService
// BlacklistService 别名 IBlacklistService
type BlacklistService = IBlacklistService
