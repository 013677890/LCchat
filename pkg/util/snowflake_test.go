package util

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenIDStringConcurrentAfterInit(t *testing.T) {
	nodeMu.Lock()
	node = nil
	nodeMu.Unlock()

	require.NoError(t, InitSnowflake(1))

	const workers = 32
	const perWorker = 64

	var wg sync.WaitGroup
	ids := make(chan string, workers*perWorker)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				ids <- GenIDString()
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[string]struct{}, workers*perWorker)
	for id := range ids {
		require.NotEmpty(t, id)
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = struct{}{}
	}
	require.Len(t, seen, workers*perWorker)
}

func TestGenIDStringPanicsWithoutInit(t *testing.T) {
	nodeMu.Lock()
	node = nil
	nodeMu.Unlock()

	require.Panics(t, func() {
		_ = GenIDString()
	})
}

func TestInitSnowflakeFromEnv(t *testing.T) {
	nodeMu.Lock()
	node = nil
	nodeMu.Unlock()
	t.Setenv(SnowflakeNodeIDEnv, "12")

	require.NoError(t, InitSnowflakeFromEnv())
	require.NotEmpty(t, GenIDString())
}

func TestInitSnowflakeFromEnvRequiresNodeID(t *testing.T) {
	nodeMu.Lock()
	node = nil
	nodeMu.Unlock()
	t.Setenv(SnowflakeNodeIDEnv, "")

	require.Error(t, InitSnowflakeFromEnv())
}

func TestGenerateRefreshToken(t *testing.T) {
	tokenA, err := GenerateRefreshToken()
	require.NoError(t, err)
	require.Len(t, tokenA, 43)

	tokenB, err := GenerateRefreshToken()
	require.NoError(t, err)
	require.Len(t, tokenB, 43)
	require.NotEqual(t, tokenA, tokenB)
}
