package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

// ProfileEventWorker 负责处理 user-service 产生的资料展示事件。
type ProfileEventWorker struct {
	worker   *outbox.Worker
	authConn *grpc.ClientConn
}

// NewProfileEventWorker 创建 user-service 的资料展示事件 Worker。
func NewProfileEventWorker(authGRPCAddr string, db *gorm.DB) (*ProfileEventWorker, error) {
	conn, err := grpc.NewClient(
		authGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			grpcx.WithInternalCaller("user-service"),
			grpcx.ClientTimeoutUnaryInterceptor(grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()}),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("user 创建 auth-service gRPC 连接失败（addr=%s）: %w", authGRPCAddr, err)
	}

	authClient := authpb.NewInternalAuthServiceClient(conn)
	worker := outbox.NewWorker(db, outbox.WorkerConfig{
		PollInterval:   time.Second,
		BatchSize:      50,
		MaxRetries:     10,
		BaseRetryDelay: time.Second,
	})

	profileWorker := &ProfileEventWorker{worker: worker, authConn: conn}
	worker.Register(accountevent.EventTypeProfileDisplayChanged, profileWorker.handleProfileDisplayChanged(authClient))
	return profileWorker, nil
}

// Start 启动资料展示事件 Worker。
func (w *ProfileEventWorker) Start(ctx context.Context) error {
	logger.Info(ctx, "User Profile Outbox Worker 启动中")
	w.worker.Start(ctx)
	return ctx.Err()
}

// Close 释放 Worker 持有的连接资源。
func (w *ProfileEventWorker) Close() error {
	if w.authConn == nil {
		return nil
	}
	return w.authConn.Close()
}

func (w *ProfileEventWorker) handleProfileDisplayChanged(client authpb.InternalAuthServiceClient) outbox.Handler {
	return func(ctx context.Context, event *outbox.Event) error {
		var payload accountevent.ProfileDisplayChangedPayload
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			return fmt.Errorf("解析 profile_display_changed 事件失败: %w", err)
		}
		if payload.UserUUID == "" {
			return errors.New("profile_display_changed 事件缺少 user_uuid")
		}

		_, err := client.UpdateLoginDisplay(ctx, &authpb.UpdateLoginDisplayRequest{
			UserUuid: payload.UserUUID,
			Nickname: payload.Nickname,
			Avatar:   payload.Avatar,
		})
		if err != nil {
			return fmt.Errorf("调用 auth-service UpdateLoginDisplay 失败: %w", err)
		}
		return nil
	}
}
