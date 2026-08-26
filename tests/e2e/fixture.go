//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// User 是端到端测试中一个真实注册账号的最小信息集合。
type User struct {
	Label     string
	Email     string
	Password  string
	Telephone string
	UUID      string
	Devices   map[string]Device
}

// Device 保存某个真实登录设备的 access/refresh token。
type Device struct {
	ID           string
	AccessToken  string
	RefreshToken string
}

// Fixture 管理一轮 E2E 测试的账号、HTTP、Redis 和 MySQL 连接。
// 测试账号使用随机后缀隔离，默认不清空用户数据库，避免误删开发环境中的其他数据。
type Fixture struct {
	t      *testing.T
	cfg    Config
	api    *HTTPClient
	redis  *redis.Client
	db     *sql.DB
	suffix string
	Users  map[string]*User
}

// NewFixture 启动一轮测试夹具，并注册 A/B/C/D 四个相互独立的真实用户。
func NewFixture(t *testing.T) *Fixture {
	t.Helper()
	if strings.TrimSpace(getenv("LCCHAT_E2E", "")) != "1" {
		t.Skip("未设置 LCCHAT_E2E=1，跳过需要 Docker 的端到端测试")
	}
	cfg := LoadConfig()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.HealthTimeout)
	t.Cleanup(cancel)
	if err := waitForHTTPHealth(ctx, cfg); err != nil {
		t.Fatalf("Docker 服务未就绪: %v", err)
	}
	if err := checkOutboxConnectorSet(ctx, cfg); err != nil {
		t.Fatalf("Kafka Connect 配置不安全: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		_ = redisClient.Close()
		t.Fatalf("Redis 不可用: %v", err)
	}
	f := &Fixture{
		t:      t,
		cfg:    cfg,
		api:    NewHTTPClient(cfg),
		redis:  redisClient,
		suffix: uuid.NewString()[:12],
		Users:  make(map[string]*User),
	}
	t.Cleanup(func() { _ = f.Close() })

	for _, label := range []string{"A", "B", "C", "D"} {
		f.Users[label] = f.register(ctx, label)
	}
	for _, login := range []struct {
		label  string
		device string
	}{
		{label: "A", device: "A1"},
		{label: "A", device: "A2"},
		{label: "B", device: "B1"},
		{label: "B", device: "B2"},
		{label: "C", device: "C1"},
		{label: "D", device: "D1"},
	} {
		f.login(ctx, f.Users[login.label], login.device)
	}
	return f
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (f *Fixture) register(ctx context.Context, label string) *User {
	f.t.Helper()
	password := "Passw0rd1"
	email := fmt.Sprintf("edge-go-%s-%s@example.com", strings.ToLower(label), f.suffix)
	telephone := fmt.Sprintf("15%09d", (int64(parseHex(f.suffix))+int64(label[0]))%1_000_000_000)
	f.seedCode(ctx, email, "111111", 1, 10*time.Minute)
	response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/register", "", "reg-"+label, map[string]any{
		"email":      email,
		"password":   password,
		"verifyCode": "111111",
		"nickname":   "GoEdge" + label + f.suffix[:4],
		"telephone":  telephone,
	})
	if err != nil {
		f.t.Fatalf("注册 %s 失败: %v", label, err)
	}
	var data map[string]any
	requireJSONData(f.t, "注册-"+label, response, &data)
	return &User{
		Label:     label,
		Email:     email,
		Password:  password,
		Telephone: telephone,
		UUID:      stringValue(data, "userUuid"),
		Devices:   make(map[string]Device),
	}
}

func (f *Fixture) login(ctx context.Context, user *User, deviceID string) Device {
	f.t.Helper()
	response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/login", "", deviceID, map[string]any{
		"account":  user.Email,
		"password": user.Password,
		"deviceInfo": map[string]any{
			"deviceName": deviceID,
			"platform":   "Web",
			"appVersion": "1.0.0",
		},
	})
	if err != nil {
		f.t.Fatalf("登录 %s/%s 失败: %v", user.Label, deviceID, err)
	}
	var data map[string]any
	requireJSONData(f.t, "登录-"+user.Label+"/"+deviceID, response, &data)
	device := Device{ID: deviceID, AccessToken: stringValue(data, "accessToken"), RefreshToken: stringValue(data, "refreshToken")}
	if device.AccessToken == "" || device.RefreshToken == "" {
		f.t.Fatalf("登录 %s/%s 未返回完整 token: %#v", user.Label, deviceID, data)
	}
	user.Devices[deviceID] = device
	return device
}

func (f *Fixture) loginByCode(ctx context.Context, user *User, deviceID, code string) Device {
	f.t.Helper()
	response, err := f.api.DoJSON(ctx, "POST", "/api/v1/public/user/login-by-code", "", deviceID, map[string]any{
		"email":      user.Email,
		"verifyCode": code,
		"deviceInfo": map[string]any{"deviceName": deviceID, "platform": "Web", "appVersion": "1.0.0"},
	})
	if err != nil {
		f.t.Fatalf("验证码登录请求失败: %v", err)
	}
	var data map[string]any
	requireJSONData(f.t, "验证码登录", response, &data)
	device := Device{ID: deviceID, AccessToken: stringValue(data, "accessToken"), RefreshToken: stringValue(data, "refreshToken")}
	user.Devices[deviceID] = device
	return device
}

func (f *Fixture) seedCode(ctx context.Context, email, code string, codeType int, ttl time.Duration) {
	f.t.Helper()
	key := fmt.Sprintf("user:verify_code:%s:%d", email, codeType)
	if err := f.redis.Set(ctx, key, code, ttl).Err(); err != nil {
		f.t.Fatalf("写入测试验证码失败 key=%s: %v", key, err)
	}
}

func (f *Fixture) closeDB() error {
	if f.db == nil {
		return nil
	}
	err := f.db.Close()
	f.db = nil
	return err
}

// database 延迟建立 MySQL 连接，只有超时撤回和数据一致性用例需要直接访问数据库。
func (f *Fixture) database(ctx context.Context) (*sql.DB, error) {
	if f.db != nil {
		return f.db, nil
	}
	db, err := sql.Open("mysql", f.cfg.MySQLDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	f.db = db
	return db, nil
}

func (f *Fixture) Close() error {
	var firstErr error
	if err := f.closeDB(); err != nil {
		firstErr = err
	}
	if f.redis != nil {
		if err := f.redis.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func parseHex(value string) int64 {
	parsed, err := strconv.ParseInt(value, 16, 64)
	if err != nil {
		return time.Now().UnixNano()
	}
	return parsed
}

func stringValue(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return value
}

func int64Value(data map[string]any, key string) int64 {
	switch value := data[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(value, 10, 64)
		return parsed
	default:
		return 0
	}
}

func mapValue(data map[string]any, key string) map[string]any {
	value, _ := data[key].(map[string]any)
	return value
}
