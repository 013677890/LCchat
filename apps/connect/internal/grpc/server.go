package grpc

import (
	"context"
	"strings"

	"github.com/013677890/LCchat-Backend/apps/connect/internal/manager"
	"github.com/013677890/LCchat-Backend/apps/connect/pb"
	msgpb "github.com/013677890/LCchat-Backend/apps/msg/pb"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"google.golang.org/protobuf/proto"
)

// Server 只保留 ConnectService 的 RPC 处理逻辑。
//
// 第二波改造后，gRPC 运行时的拦截器链、监听器与停机流程
// 统一由 pkg/grpcx.NewServer / Run / GracefulStop 负责，
// 这里不再重复包一层自定义 runtime。
type Server struct {
	pb.UnimplementedConnectServiceServer
	connManager *manager.ConnectionManager
}

// NewServer 创建 connect gRPC 业务处理器。
func NewServer(connManager *manager.ConnectionManager) *Server {
	return &Server{connManager: connManager}
}

// PushToDevice 向指定设备投递消息。
func (s *Server) PushToDevice(ctx context.Context, req *pb.PushToDeviceRequest) (*pb.PushToDeviceResponse, error) {
	data, err := marshalEnvelope(req.Message)
	if err != nil {
		logger.Warn(ctx, "向设备投递前序列化消息失败", logger.ErrorField("error", err))
		return &pb.PushToDeviceResponse{Delivered: false}, nil
	}
	delivery := deliveryMetadata(ctx, req.Message)
	delivered := s.connManager.SendToDevice(req.UserUuid, req.DeviceId, data, delivery)
	return &pb.PushToDeviceResponse{Delivered: delivered}, nil
}

// PushToUser 向用户所有在线设备广播消息。
func (s *Server) PushToUser(ctx context.Context, req *pb.PushToUserRequest) (*pb.PushToUserResponse, error) {
	data, err := marshalEnvelope(req.Message)
	if err != nil {
		logger.Warn(ctx, "向用户广播前序列化消息失败", logger.ErrorField("error", err))
		return &pb.PushToUserResponse{DeliveredCount: 0}, nil
	}
	delivery := deliveryMetadata(ctx, req.Message)
	count := s.connManager.SendToUser(req.UserUuid, data, delivery)
	return &pb.PushToUserResponse{DeliveredCount: int32(count)}, nil
}

// BroadcastToUsers 向多个用户广播相同消息。
func (s *Server) BroadcastToUsers(ctx context.Context, req *pb.BroadcastToUsersRequest) (*pb.BroadcastToUsersResponse, error) {
	data, err := marshalEnvelope(req.Message)
	if err != nil {
		logger.Warn(ctx, "批量广播前序列化消息失败", logger.ErrorField("error", err))
		return &pb.BroadcastToUsersResponse{}, nil
	}
	delivery := deliveryMetadata(ctx, req.Message)
	var successCount, totalDelivered int32
	for _, userUUID := range req.UserUuids {
		count := s.connManager.SendToUser(userUUID, data, delivery)
		if count > 0 {
			successCount++
			totalDelivered += int32(count)
		}
	}
	return &pb.BroadcastToUsersResponse{SuccessCount: successCount, TotalDelivered: totalDelivered}, nil
}

func marshalEnvelope(message *pb.MessageEnvelope) ([]byte, error) {
	if message == nil {
		return nil, nil
	}
	return proto.Marshal(message)
}

func deliveryMetadata(ctx context.Context, message *pb.MessageEnvelope) manager.DeliveryMetadata {
	if message == nil || !message.GetAckRequired() || message.GetSeq() <= 0 {
		return manager.DeliveryMetadata{}
	}

	metadata := manager.DeliveryMetadata{
		Seq:         message.GetSeq(),
		AckRequired: true,
	}
	if message.GetType() != "MSG_PUSH" {
		return metadata
	}

	var item msgpb.MsgItem
	if err := proto.Unmarshal(message.GetData(), &item); err != nil {
		logger.Warn(ctx, "解析下行消息 ACK 水位失败",
			logger.String("message_type", message.GetType()),
			logger.Int64("seq", message.GetSeq()),
			logger.ErrorField("error", err),
		)
		return metadata
	}
	metadata.ConvID = strings.TrimSpace(item.GetConvId())
	return metadata
}

// KickConnection 主动断开指定设备连接。
func (s *Server) KickConnection(ctx context.Context, req *pb.KickConnectionRequest) (*pb.KickConnectionResponse, error) {
	success := s.connManager.KickDevice(req.UserUuid, req.DeviceId)
	if success {
		logger.Info(ctx, "主动断开连接成功",
			logger.String("user_uuid", req.UserUuid),
			logger.String("device_id", req.DeviceId),
			logger.String("reason", req.Reason),
		)
	}
	return &pb.KickConnectionResponse{Success: success}, nil
}
