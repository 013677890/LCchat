package util

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenIDStringConcurrentLazyInit(t *testing.T) {
	nodeMu.Lock()
	node = nil
	nodeMu.Unlock()

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
