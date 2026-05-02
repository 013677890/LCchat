package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	userpb "github.com/013677890/LCchat-Backend/apps/user/pb"
	"github.com/013677890/LCchat-Backend/pkg/accountevent"
	"github.com/013677890/LCchat-Backend/pkg/grpcx"
	"github.com/013677890/LCchat-Backend/pkg/kafka"
	"github.com/013677890/LCchat-Backend/pkg/logger"
	"github.com/013677890/LCchat-Backend/pkg/outbox"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

// AccountEventWorker 负责处理 auth-service 产生的账户领域 Outbox 事件。
type AccountEventWorker struct {
	worker                 *outbox.Worker
	userConn               *grpc.ClientConn
	accountDeletedProducer *kafka.Producer
}

// NewAccountEventWorker 创建 auth-service 的账户事件 Worker。
func NewAccountEventWorker(userGRPCAddr string, db *gorm.DB, brokers []string, accountDeletedTopic string, log *zap.Logger) (*AccountEventWorker, error) {
	conn, err := grpc.NewClient(
		userGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			grpcx.WithInternalCaller("auth-service"),
			grpcx.ClientTimeoutUnaryInterceptor(grpcx.ClientTimeoutConfig{MethodTimeouts: grpcx.DefaultClientMethodTimeouts()}),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("auth 创建 user-service gRPC 连接失败（addr=%s）: %w", userGRPCAddr, err)
	}

	profileClient := userpb.NewInternalProfileServiceClient(conn)
	producer := kafka.NewProducer(brokers, accountDeletedTopic)
	worker := outbox.NewWorker(db, outbox.WorkerConfig{
		PollInterval:   time.Second,
		BatchSize:      50,
		MaxRetries:     10,
		BaseRetryDelay: time.Second,
	})

	accountWorker := &AccountEventWorker{
		worker:                 worker,
		userConn:               conn,
		accountDeletedProducer: producer,
	}
	worker.Register(accountevent.EventTypeUserCreated, accountWorker.handleUserCreated(profileClient))
	worker.Register(accountevent.EventTypeAccountDeleted, accountWorker.handleAccountDeleted())
	_ = log
	return accountWorker, nil
}

// Start 启动账户事件 Worker。
func (w *AccountEventWorker) Start(ctx context.Context) error {
	logger.Info(ctx, "Auth Outbox Worker 启动中")
	w.worker.Start(ctx)
	return ctx.Err()
}

// Close 释放 Worker 持有的外部资源。
func (w *AccountEventWorker) Close() error {
	var errs []error
	if w.accountDeletedProducer != nil {
		if err := w.accountDeletedProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 account.deleted Producer 失败: %w", err))
		}
	}
	if w.userConn != nil {
		if err := w.userConn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 user-service gRPC 连接失败: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (w *AccountEventWorker) handleUserCreated(client userpb.InternalProfileServiceClient) outbox.Handler {
	return func(ctx context.Context, event *outbox.Event) error {
		var payload accountevent.UserCreatedPayload
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			return fmt.Errorf("解析 user_created 事件失败: %w", err)
		}
		if payload.UserUUID == "" {
			return errors.New("user_created 事件缺少 user_uuid")
		}

		_, err := client.CreateProfile(ctx, &userpb.CreateProfileRequest{
			UserUuid: payload.UserUUID,
			Nickname: payload.Nickname,
			Avatar:   payload.Avatar,
		})
		if err != nil {
			return fmt.Errorf("调用 user-service CreateProfile 失败: %w", err)
		}
		return nil
	}
}

func (w *AccountEventWorker) handleAccountDeleted() outbox.Handler {
	return func(ctx context.Context, event *outbox.Event) error {
		var payload accountevent.AccountDeletedPayload
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			return fmt.Errorf("解析 account_deleted 事件失败: %w", err)
		}
		if payload.UserUUID == "" {
			return errors.New("account_deleted 事件缺少 user_uuid")
		}

		if err := w.accountDeletedProducer.SendWithKey(ctx, []byte(payload.UserUUID), []byte(event.Payload)); err != nil {
			return fmt.Errorf("投递 account.deleted Kafka 事件失败: %w", err)
		}
		return nil
	}
}
