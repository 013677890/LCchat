package _achieve
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"ChatServer/apps/gateway/internal/dto"
	"ChatServer/apps/gateway/internal/service"
	"ChatServer/consts"
	"ChatServer/pkg/logger"
)

// ==================== Mock 定义 ====================

// MockLoginService 是 LoginService 的 Mock 实现
type MockLoginService struct {
	mockLogin func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error)
}

func (m *MockLoginService) Login(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
	return m.mockLogin(ctx, req, deviceID)
}

// ==================== 测试 ====================

// init 初始化 logger（测试模式，不输出日志）
func init() {
	// 创建一个 development 模式的 logger，输出到 discard
	testLogger := zap.NewNop() // 无操作 logger，完全静默
	logger.ReplaceGlobal(testLogger)
}

func TestLoginHandler_Login(t *testing.T) {
	// 1. 设置 Gin 环境（测试模式，不输出日志）
	gin.SetMode(gin.TestMode)

	// 2. 表格驱动测试
	tests := []struct {
		name           string
		reqBody        interface{}
		headers        map[string]string
		mockLogin      func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error)
		expectedStatus int
		expectedCode   int32
	}{
		{
			name: "登录成功",
			reqBody: dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "password123",
				DeviceInfo: dto.DeviceInfo{
					Platform:   "iOS",
					OSVersion:  "17.0",
					AppVersion: "1.0.0",
				},
			},
			headers: map[string]string{
				"Content-Type": "application/json",
				"X-Device-ID":  "test-device-001",
			},
			mockLogin: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
				return &dto.LoginResponse{
					AccessToken:  "test-access-token",
					RefreshToken: "test-refresh-token",
					TokenType:    "Bearer",
					ExpiresIn:    7200,
					UserInfo: dto.UserInfo{
						UUID:      "550e8400-e29b-41d4-a716-446655440000", // 标准 UUID 格式
						Nickname:  "测试用户",
						Telephone: "13800138000",
						Email:     "test@example.com",
					},
				}, nil
			},
			expectedStatus: http.StatusOK,
			expectedCode:   consts.CodeSuccess,
		},
		{
			name: "登录成功（无设备ID，自动生成）",
			reqBody: dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "password123",
			},
			headers: map[string]string{
				"Content-Type": "application/json",
				// 注意：这里没有 X-Device-ID，Handler 应该自动生成
			},
			mockLogin: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
				// 验证 deviceId 不为空
				assert.NotEmpty(t, deviceID, "deviceId 不应该为空")
				return &dto.LoginResponse{
					AccessToken:  "test-access-token",
					RefreshToken: "test-refresh-token",
					TokenType:    "Bearer",
					ExpiresIn:    7200,
					UserInfo: dto.UserInfo{
						UUID:      "550e8400-e29b-41d4-a716-446655440000", // 标准 UUID 格式
						Nickname:  "测试用户",
						Telephone: "13800138000",
					},
				}, nil
			},
			expectedStatus: http.StatusOK,
			expectedCode:   consts.CodeSuccess,
		},
		{
			name: "密码错误",
			reqBody: dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "wrongpassword",
			},
			headers: map[string]string{
				"Content-Type": "application/json",
				"X-Device-ID":  "test-device-002",
			},
			mockLogin: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
				return nil, &service.BusinessError{
					Code:    consts.CodePasswordError,
					Message: "密码错误",
				}
			},
			expectedStatus: http.StatusOK,
			expectedCode:   consts.CodePasswordError,
		},
		{
			name: "用户不存在",
			reqBody: dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "password123",
			},
			headers: map[string]string{
				"Content-Type": "application/json",
				"X-Device-ID":  "test-device-003",
			},
			mockLogin: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
				return nil, &service.BusinessError{
					Code:    consts.CodeUserNotFound,
					Message: "用户不存在",
				}
			},
			expectedStatus: http.StatusOK,
			expectedCode:   consts.CodeUserNotFound,
		},
		{
			name: "Service 内部错误（非 BusinessError 类型）",
			reqBody: dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "password123",
			},
			headers: map[string]string{
				"Content-Type": "application/json",
				"X-Device-ID":  "test-device-004",
			},
			mockLogin: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
				return nil, errors.New("内部错误")
			},
			expectedStatus: http.StatusInternalServerError, // 内部错误返回 500
			expectedCode:   consts.CodeInternalError,
		},
		{
			name: "参数错误（手机号格式错误）",
			reqBody: dto.LoginRequest{
				Telephone: "123", // 不是 11 位
				Password:  "password123",
			},
			headers: map[string]string{
				"Content-Type": "application/json",
				"X-Device-ID":  "test-device-005",
			},
			mockLogin: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
				// 参数验证失败，不应该调用 Service
				assert.Fail(t, "不应该调用 Service")
				return nil, nil
			},
			expectedStatus: http.StatusOK,
			expectedCode:   consts.CodeParamError,
		},
		{
			name: "参数错误（密码长度不足）",
			reqBody: dto.LoginRequest{
				Telephone: "13800138000",
				Password:  "1234567", // 少于 8 位
			},
			headers: map[string]string{
				"Content-Type": "application/json",
				"X-Device-ID":  "test-device-006",
			},
			mockLogin: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
				// 参数验证失败，不应该调用 Service
				assert.Fail(t, "不应该调用 Service")
				return nil, nil
			},
			expectedStatus: http.StatusOK,
			expectedCode:   consts.CodeParamError,
		},
		{
			name:    "请求体格式错误",
			reqBody: "invalid json",
			headers: map[string]string{
				"Content-Type": "application/json",
				"X-Device-ID":  "test-device-007",
			},
			mockLogin: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
				// JSON 解析失败，不应该调用 Service
				assert.Fail(t, "不应该调用 Service")
				return nil, nil
			},
			expectedStatus: http.StatusOK,
			expectedCode:   consts.CodeParamError, // JSON 解析失败也会返回参数错误
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A. 创建 Mock Service
			mockLoginSvc := &MockLoginService{
				mockLogin: tt.mockLogin,
			}

			// B. 创建被测 Handler (依赖注入)
			loginHandler := NewLoginHandler(mockLoginSvc)

			// C. 构建 HTTP 请求
			w := httptest.NewRecorder()
			var jsonBody []byte
			var err error

			if str, ok := tt.reqBody.(string); ok {
				jsonBody = []byte(str)
			} else {
				jsonBody, err = json.Marshal(tt.reqBody)
				assert.NoError(t, err, "JSON 编码失败")
			}

			req, err := http.NewRequest("POST", "/api/v1/public/login", bytes.NewBuffer(jsonBody))
			assert.NoError(t, err, "创建请求失败")

			// 设置请求头
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			// D. 模拟 Gin 上下文并执行 Handler
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// 直接调用 Handler 方法
			loginHandler.Login(c)

			// E. 断言响应状态码
			assert.Equal(t, tt.expectedStatus, w.Code, "HTTP 状态码不匹配")

			// F. 解析响应 body 验证内容
			var response struct {
				Code    int32       `json:"code"`
				Message string      `json:"message"`
				Data    interface{} `json:"data"`
			}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err, "解析响应体失败")

			// 验证响应中的 code
			assert.Equal(t, tt.expectedCode, response.Code, "响应 code 不匹配")
		})
	}
}

// BenchmarkLoginHandler_Login 基准测试
func BenchmarkLoginHandler_Login(b *testing.B) {
	gin.SetMode(gin.TestMode)

	mockLoginSvc := &MockLoginService{
		mockLogin: func(ctx context.Context, req *dto.LoginRequest, deviceID string) (*dto.LoginResponse, error) {
			return &dto.LoginResponse{
				AccessToken:  "test-access-token",
				RefreshToken: "test-refresh-token",
				TokenType:    "Bearer",
				ExpiresIn:    7200,
				UserInfo: dto.UserInfo{
					UUID:      "550e8400-e29b-41d4-a716-446655440000", // 标准 UUID 格式
					Nickname:  "测试用户",
					Telephone: "13800138000",
				},
			}, nil
		},
	}

	loginHandler := NewLoginHandler(mockLoginSvc)

	reqBody := dto.LoginRequest{
		Telephone: "13800138000",
		Password:  "password123",
	}

	jsonBody, _ := json.Marshal(reqBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/v1/public/login", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Device-ID", "test-device-001")

		c, _ := gin.CreateTestContext(w)
		c.Request = req

		loginHandler.Login(c)
	}
}