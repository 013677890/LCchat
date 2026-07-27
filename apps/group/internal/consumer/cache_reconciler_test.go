package consumer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/013677890/LCchat-Backend/apps/group/internal/repository"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCacheReconcileRepository struct {
	targets         []repository.GroupCacheReconcileTarget
	reconcileErrors map[string]error
	reconciled      []string
}

func (f *fakeCacheReconcileRepository) ApplyGroupCacheEvent(
	context.Context,
	groupevent.GroupCacheEventPayload,
) error {
	return nil
}

func (f *fakeCacheReconcileRepository) ReconcileGroupCache(_ context.Context, groupUUID string) error {
	f.reconciled = append(f.reconciled, groupUUID)
	return f.reconcileErrors[groupUUID]
}

func (f *fakeCacheReconcileRepository) ListGroupCacheReconcileTargets(
	_ context.Context,
	afterID int64,
	limit int,
) ([]repository.GroupCacheReconcileTarget, error) {
	result := make([]repository.GroupCacheReconcileTarget, 0, limit)
	for _, target := range f.targets {
		if target.ID <= afterID {
			continue
		}
		result = append(result, target)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func TestCacheReconcilerRunOnceScansAllBatchesAndContinuesAfterGroupError(t *testing.T) {
	expectedErr := errors.New("redis unavailable")
	repo := &fakeCacheReconcileRepository{
		targets: []repository.GroupCacheReconcileTarget{
			{ID: 1, GroupUUID: "group-1"},
			{ID: 2, GroupUUID: "group-2"},
			{ID: 3, GroupUUID: "group-3"},
		},
		reconcileErrors: map[string]error{"group-2": expectedErr},
	}
	reconciler, err := NewCacheReconciler(repo, CacheReconcilerConfig{
		Interval:  time.Minute,
		BatchSize: 2,
	})
	require.NoError(t, err)

	err = reconciler.RunOnce(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []string{"group-1", "group-2", "group-3"}, repo.reconciled)
}

func TestNewCacheReconcilerRejectsInvalidExplicitConfig(t *testing.T) {
	repo := &fakeCacheReconcileRepository{}
	_, err := NewCacheReconciler(repo, CacheReconcilerConfig{Interval: 0, BatchSize: 1})
	assert.Error(t, err)
	_, err = NewCacheReconciler(repo, CacheReconcilerConfig{Interval: time.Minute, BatchSize: 0})
	assert.Error(t, err)
}

func TestCacheReconcilerRunOnceBoundsErrorsDuringGlobalFailure(t *testing.T) {
	expectedErr := errors.New("redis unavailable")
	repo := &fakeCacheReconcileRepository{
		reconcileErrors: make(map[string]error),
	}
	total := maxCacheReconcileErrorSamples + 5
	for index := 1; index <= total; index++ {
		groupUUID := fmt.Sprintf("group-%02d", index)
		repo.targets = append(repo.targets, repository.GroupCacheReconcileTarget{
			ID:        int64(index),
			GroupUUID: groupUUID,
		})
		repo.reconcileErrors[groupUUID] = expectedErr
	}
	reconciler, err := NewCacheReconciler(repo, CacheReconcilerConfig{
		Interval:  time.Minute,
		BatchSize: 4,
	})
	require.NoError(t, err)

	err = reconciler.RunOnce(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
	assert.Len(t, repo.reconciled, total, "错误样本有界不能改变继续扫描全部群的语义")
	assert.Equal(t, maxCacheReconcileErrorSamples, strings.Count(err.Error(), "reconcile group "))
	assert.Contains(t, err.Error(), "5 additional group cache reconcile errors omitted")
}
