package redisretry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSendRedisTaskReportsMissingProducer(t *testing.T) {
	previous := getGlobalProducer()
	SetGlobalProducer(nil)
	t.Cleanup(func() { SetGlobalProducer(previous) })

	err := sendRedisTask(context.Background(), BuildDelTask("cache:1"))
	require.ErrorIs(t, err, errProducerNotConfigured)
}
