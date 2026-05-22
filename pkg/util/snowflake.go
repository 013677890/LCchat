package util

import (
	"fmt"
	"sync"

	"github.com/bwmarrin/snowflake"
)

var (
	node   *snowflake.Node
	nodeMu sync.Mutex
)

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

func currentSnowflakeNode() *snowflake.Node {
	nodeMu.Lock()
	defer nodeMu.Unlock()
	if node == nil {
		// 如果未手动初始化，默认使用节点 1。
		defaultNode, err := snowflake.NewNode(1)
		if err != nil {
			panic(fmt.Errorf("failed to create default snowflake node: %w", err))
		}
		node = defaultNode
	}
	return node
}
