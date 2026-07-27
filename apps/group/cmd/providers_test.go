package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvideGroupCacheReconcilerConfigStrictParsing(t *testing.T) {
	t.Run("默认值", func(t *testing.T) {
		t.Setenv("GROUP_CACHE_RECONCILE_INTERVAL", "")
		t.Setenv("GROUP_CACHE_RECONCILE_BATCH_SIZE", "")
		cfg, err := provideGroupCacheReconcilerConfig()
		require.NoError(t, err)
		assert.Equal(t, 6*time.Hour, cfg.Interval)
		assert.Equal(t, 100, cfg.BatchSize)
	})

	t.Run("显式合法值", func(t *testing.T) {
		t.Setenv("GROUP_CACHE_RECONCILE_INTERVAL", "30s")
		t.Setenv("GROUP_CACHE_RECONCILE_BATCH_SIZE", "25")
		cfg, err := provideGroupCacheReconcilerConfig()
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, cfg.Interval)
		assert.Equal(t, 25, cfg.BatchSize)
	})

	for name, value := range map[string]string{
		"裸秒数不兼容": "30",
		"零值":     "0s",
		"负值":     "-1m",
		"非法文本":   "ten-minutes",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GROUP_CACHE_RECONCILE_INTERVAL", value)
			t.Setenv("GROUP_CACHE_RECONCILE_BATCH_SIZE", "100")
			_, err := provideGroupCacheReconcilerConfig()
			assert.Error(t, err)
		})
	}
}
