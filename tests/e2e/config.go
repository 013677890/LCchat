//go:build e2e

// Package e2e 包含需要连接 Docker Compose 运行环境的黑盒与端到端测试。
//
// 这些测试故意不放进普通的 go test ./... 流程：它们会创建真实账号、写入
// Redis、访问 MySQL，并且部分生命周期用例会重启 Connect 或暂停 Redis。
// 运行时需要显式添加 -tags=e2e，并设置 LCCHAT_E2E=1。
package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Config 集中保存 Docker 本地联调入口和测试超时。
// 所有地址都支持环境变量覆盖，便于切换到 k3s port-forward 或 CI 服务。
type Config struct {
	GatewayBase      string
	ConnectBase      string
	ConnectWSBase    string
	ConnectWSOrigin  string
	RedisAddr        string
	MySQLDSN         string
	RequestTimeout   time.Duration
	EventTimeout     time.Duration
	HealthTimeout    time.Duration
	ComposeProject   string
	ComposeDirectory string
}

// LoadConfig 读取测试配置，并给出与项目本地 Docker Compose 一致的默认值。
func LoadConfig() Config {
	return Config{
		GatewayBase:      envString("LCCHAT_BASE_URL", "http://127.0.0.1:8080"),
		ConnectBase:      envString("LCCHAT_CONNECT_HTTP_BASE_URL", "http://127.0.0.1:8081"),
		ConnectWSBase:    envString("LCCHAT_CONNECT_WS_BASE_URL", "ws://127.0.0.1:8081"),
		ConnectWSOrigin:  os.Getenv("LCCHAT_CONNECT_WS_ORIGIN"),
		RedisAddr:        envString("LCCHAT_REDIS_ADDR", "127.0.0.1:16379"),
		MySQLDSN:         envString("LCCHAT_MYSQL_DSN", "root:root@tcp(127.0.0.1:13306)/chat_server?parseTime=true&multiStatements=true"),
		RequestTimeout:   envDuration("LCCHAT_REQUEST_TIMEOUT", 10*time.Second),
		EventTimeout:     envDuration("LCCHAT_EVENT_TIMEOUT", 12*time.Second),
		HealthTimeout:    envDuration("LCCHAT_HEALTH_TIMEOUT", 60*time.Second),
		ComposeProject:   os.Getenv("COMPOSE_PROJECT_NAME"),
		ComposeDirectory: envString("LCCHAT_ROOT", repositoryRoot()),
	}
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// repositoryRoot 根据当前源码位置推导仓库根目录，避免 go test 的工作目录变化
// 导致 docker compose 命令找不到 compose 文件。
func repositoryRoot() string {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
}

// waitForHTTPHealth 等待 Gateway 和 Connect 的健康接口都可访问。
// Docker 服务启动后还要等待 gRPC 客户端和 Kafka consumer 初始化，因此不能只执行一次请求。
func waitForHTTPHealth(ctx context.Context, cfg Config) error {
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(cfg.HealthTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := checkHealth(ctx, client, strings.TrimRight(cfg.GatewayBase, "/")+"/health"); err != nil {
			lastErr = err
		} else if err := checkHealth(ctx, client, strings.TrimRight(cfg.ConnectBase, "/")+"/health"); err != nil {
			lastErr = err
		} else {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("health check timeout")
	}
	return fmt.Errorf("等待 Docker 服务健康检查超时: %w", lastErr)
}

func checkHealth(ctx context.Context, client *http.Client, endpoint string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s 返回 HTTP %d", endpoint, response.StatusCode)
	}
	return nil
}

// runCompose 用于少数必须改变运行时状态的生命周期用例，例如 Connect 重启和
// Redis 不可用 ACK。命令失败会把完整输出带回测试，方便定位 Docker 状态问题。
func runCompose(ctx context.Context, cfg Config, args ...string) error {
	commandArgs := make([]string, 0, len(args)+2)
	commandArgs = append(commandArgs, "compose")
	if cfg.ComposeProject != "" {
		commandArgs = append(commandArgs, "-p", cfg.ComposeProject)
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(ctx, "docker", commandArgs...)
	command.Dir = cfg.ComposeDirectory
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose %s 失败: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func parseIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
