package util

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/bwmarrin/snowflake"
)

const (
	SnowflakeNodeIDEnv = "SNOWFLAKE_NODE_ID"
	refreshTokenBytes  = 32
)

var (
	node   *snowflake.Node
	nodeMu sync.Mutex
)

// InitSnowflakeFromEnv initializes the process-wide Snowflake node from SNOWFLAKE_NODE_ID.
func InitSnowflakeFromEnv() error {
	raw := strings.TrimSpace(os.Getenv(SnowflakeNodeIDEnv))
	if raw == "" {
		return fmt.Errorf("%s is required", SnowflakeNodeIDEnv)
	}

	machineID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", SnowflakeNodeIDEnv, raw, err)
	}

	return InitSnowflake(machineID)
}

// InitSnowflake 初始化雪花算法节点
// machineID: 机器 ID (0-1023)
func InitSnowflake(machineID int64) error {
	newNode, err := snowflake.NewNode(machineID)
	if err != nil {
		return fmt.Errorf("failed to create snowflake node: %w", err)
	}
	nodeMu.Lock()
	node = newNode
	nodeMu.Unlock()
	return nil
}

// GenID 生成一个全局唯一的 int64 ID
func GenID() int64 {
	return currentSnowflakeNode().Generate().Int64()
}

// GenIDString 生成一个全局唯一的字符串 ID
func GenIDString() string {
	return currentSnowflakeNode().Generate().String()
}

// GenerateRefreshToken creates an opaque device refresh credential.
func GenerateRefreshToken() (string, error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func currentSnowflakeNode() *snowflake.Node {
	nodeMu.Lock()
	defer nodeMu.Unlock()
	if node == nil {
		panic(fmt.Errorf("snowflake node is not initialized; set %s and call InitSnowflakeFromEnv before generating IDs", SnowflakeNodeIDEnv))
	}
	return node
}
