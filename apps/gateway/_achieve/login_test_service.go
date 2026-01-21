package _achieve


import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ChatServer/apps/gateway/internal/dto"
	"ChatServer/apps/gateway/internal/mocks"
	"ChatServer/apps/user/pb"
	"ChatServer/consts"
	"ChatServer/pkg/logger"
)

// init 初始化 logger（测试模式，不输出日志）
func init() {
	// 创建一个 development 模式的 logger，输出到 discard
	// 这样测试时不会有日志输出干扰
	testLogger := zap.NewNop() // 无操作 logger，完全静默
	logger.ReplaceGlobal(testLogger)
}

func TestLoginService_Login(t *testing.T) {
	// 1. 初始化 Mock 控制器
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// 2. 创建 gRPC Client 的 Mock 对象
	mockUserClient := mocks.NewMockUserServiceClient(ctrl)

	// 3. 创建被测 Service (依赖注入)
	loginService := NewLoginService(mockUserClient)

	// 4. 表格驱动测试
	tests := []struct {
		name          string
		req           *dto.LoginRequest
		deviceID      string
		setupMock     func()
		wantResp      *dto.LoginResponse
		wantErr       bool
		expectErrCode int32
	}{
		{
			name: "正常登录成功",
			req: &dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "password123",
				DeviceInfo: dto.DeviceInfo{
					Platform:    "iOS",
					OSVersion:   "17.0",
					AppVersion:  "1.0.0",
					DeviceModel: "iPhone 15",
				},
			},
			deviceID: "test-device-001",
			setupMock: func() {
				// 期望 gRPC.Login 被调用，并返回成功数据
				mockUserClient.EXPECT().
					Login(gomock.Any(), gomock.Any()).
					Return(&pb.LoginResponse{
						UserInfo: &pb.UserInfo{
							Uuid:      "user-uuid-001",
							Nickname:  "测试用户",
							Telephone: "13800138000",
							Email:     "test@example.com",
							Avatar:    "http://avatar.example.com",
							Gender:    1,
							Signature: "这是签名",
							Birthday:  "1990-01-01",
						},
					}, nil)
			},
			wantResp: &dto.LoginResponse{
				TokenType: "Bearer",
				UserInfo: dto.UserInfo{
					UUID:      "user-uuid-001",
					Nickname:  "测试用户",
					Telephone: "13800138000",
					Email:     "test@example.com",
					Avatar:    "http://avatar.example.com",
					Gender:    1,
					Signature: "这是签名",
					Birthday:  "1990-01-01",
				},
			},
			wantErr: false,
		},
		{
			name: "密码错误",
			req: &dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "wrongpassword",
			},
			deviceID: "test-device-002",
			setupMock: func() {
				// 期望 gRPC.Login 被调用，并返回密码错误
				mockUserClient.EXPECT().
					Login(gomock.Any(), gomock.Any()).
					Return(nil, status.Error(codes.InvalidArgument, "密码错误"))
			},
			wantResp:      nil,
			wantErr:       true,
			expectErrCode: consts.CodeParamError,
		},
		{
			name: "用户不存在",
			req: &dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "password123",
			},
			deviceID: "test-device-003",
			setupMock: func() {
				// 期望 gRPC.Login 被调用，并返回用户不存在
				mockUserClient.EXPECT().
					Login(gomock.Any(), gomock.Any()).
					Return(nil, status.Error(codes.NotFound, "用户不存在"))
			},
			wantResp:      nil,
			wantErr:       true,
			expectErrCode: consts.CodeUserNotFound,
		},
		{
			name: "gRPC 调用失败（网络错误）",
			req: &dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "password123",
			},
			deviceID: "test-device-004",
			setupMock: func() {
				// 期望 gRPC.Login 被调用，并返回网络错误
				mockUserClient.EXPECT().
					Login(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("connection refused"))
			},
			wantResp:      nil,
			wantErr:       true,
			expectErrCode: consts.CodeInternalError,
		},
		{
			name: "gRPC 返回用户信息为空",
			req: &dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "password123",
			},
			deviceID: "test-device-005",
			setupMock: func() {
				// 期望 gRPC.Login 被调用，但 UserInfo 为空
				mockUserClient.EXPECT().
					Login(gomock.Any(), gomock.Any()).
					Return(&pb.LoginResponse{
						UserInfo: nil, // 用户信息为空
					}, nil)
			},
			wantResp:      nil,
			wantErr:       true,
			expectErrCode: consts.CodeInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A. 设置 Mock 行为
			tt.setupMock()

			// B. 调用被测方法
			ctx := context.Background()
			resp, err := loginService.Login(ctx, tt.req, tt.deviceID)

			// C. 断言结果
			if tt.wantErr {
				assert.Error(t, err, "期望返回错误")
				assert.Nil(t, resp, "错误情况下 resp 应该为 nil")

				// 检查错误类型和错误码
				if tt.expectErrCode != 0 {
					bizErr, ok := err.(*BusinessError)
					assert.True(t, ok, "错误应该是 BusinessError 类型")
					if ok {
						assert.Equal(t, tt.expectErrCode, bizErr.Code, "错误码不匹配")
					}
				}
			} else {
				assert.NoError(t, err, "期望无错误")
				assert.NotNil(t, resp, "成功情况下 resp 不应该为 nil")

				// 验证 Token 相关字段
				assert.NotEmpty(t, resp.AccessToken, "AccessToken 不应该为空")
				assert.NotEmpty(t, resp.RefreshToken, "RefreshToken 不应该为空")
				assert.Equal(t, "Bearer", resp.TokenType, "TokenType 应该是 Bearer")
				assert.Greater(t, resp.ExpiresIn, int64(0), "ExpiresIn 应该大于 0")

				// 验证用户信息
				assert.Equal(t, tt.wantResp.UserInfo.UUID, resp.UserInfo.UUID, "UUID 不匹配")
				assert.Equal(t, tt.wantResp.UserInfo.Nickname, resp.UserInfo.Nickname, "Nickname 不匹配")
				assert.Equal(t, tt.wantResp.UserInfo.Telephone, resp.UserInfo.Telephone, "Telephone 不匹配")
			}
		})
	}
}

// BenchmarkLoginService_Login 基准测试
func BenchmarkLoginService_Login(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockUserClient := mocks.NewMockUserServiceClient(ctrl)
	loginService := NewLoginService(mockUserClient)

	// 设置 Mock 行为（只设置一次，因为基准测试会重复调用）
	mockUserClient.EXPECT().
		Login(gomock.Any(), gomock.Any()).
		Return(&pb.LoginResponse{
			UserInfo: &pb.UserInfo{
				Uuid:      "user-uuid-001",
				Nickname:  "测试用户",
				Telephone: "13800138000",
				Email:     "test@example.com",
			},
		}, nil).
		AnyTimes() // 允许被调用任意次数

	req := &dto.LoginRequest{
		Telephone: "13800138000",
		Password:  "password123",
	}

	ctx := context.Background()
	deviceID := "test-device-001"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loginService.Login(ctx, req, deviceID)
	}
}