package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/apps/msg/internal/domain/conversation"
	"github.com/013677890/LCchat-Backend/pkg/groupevent"
	"github.com/013677890/LCchat-Backend/pkg/kafka"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGroupMembershipProjectorRepository struct {
	applyFn func(context.Context, groupevent.GroupCacheEventPayload) error
	payload groupevent.GroupCacheEventPayload
	calls   int
}

func (f *fakeGroupMembershipProjectorRepository) ApplyGroupCacheEvent(
	ctx context.Context,
	payload groupevent.GroupCacheEventPayload,
) error {
	f.calls++
	f.payload = payload
	if f.applyFn != nil {
		return f.applyFn(ctx, payload)
	}
	return nil
}

func validProjectorMessage(t *testing.T) []byte {
	t.Helper()
	encoded, err := groupevent.Encode(groupevent.GroupCacheEventPayload{
		SchemaVersion:     groupevent.GroupCacheSchemaVersion,
		ProjectionVersion: 1,
		EventID:           "event-1",
		Action:            groupevent.ActionJoinRequestReviewed,
		GroupUUID:         "group-1",
		JoinRequest: &groupevent.GroupJoinRequestSnapshot{
			ApplyID:         1,
			ApplicantUUID:   "user-1",
			CreatedAtUnixMs: 1710000000123,
		},
	})
	require.NoError(t, err)
	return []byte(encoded)
}

func TestGroupMembershipProjectorHandleProjectsCurrentStrictPayload(t *testing.T) {
	repo := &fakeGroupMembershipProjectorRepository{}
	projector := &GroupMembershipProjector{repo: repo}

	require.NoError(t, projector.handle(context.Background(), validProjectorMessage(t)))
	assert.Equal(t, 1, repo.calls)
	assert.Equal(t, "event-1", repo.payload.EventID)
	assert.Equal(t, int64(1), repo.payload.ProjectionVersion)
}

func TestGroupMembershipProjectorHandleParksContractAndVersionErrors(t *testing.T) {
	for name, applyErr := range map[string]error{
		"投影内容非法": conversation.ErrInvalidGroupProjectionEvent,
		"单群版本缺口": conversation.ErrGroupProjectionVersionGap,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeGroupMembershipProjectorRepository{
				applyFn: func(context.Context, groupevent.GroupCacheEventPayload) error {
					return applyErr
				},
			}
			projector := &GroupMembershipProjector{repo: repo}

			err := projector.handle(context.Background(), validProjectorMessage(t))
			assert.Error(t, err)
			assert.True(t, kafka.IsPermanent(err))
		})
	}
}

func TestGroupMembershipProjectorHandleRetriesDatabaseError(t *testing.T) {
	repo := &fakeGroupMembershipProjectorRepository{
		applyFn: func(context.Context, groupevent.GroupCacheEventPayload) error {
			return errors.New("mysql unavailable")
		},
	}
	projector := &GroupMembershipProjector{repo: repo}

	err := projector.handle(context.Background(), validProjectorMessage(t))
	assert.Error(t, err)
	assert.False(t, kafka.IsPermanent(err))
}

func TestGroupMembershipProjectorHandleRejectsLegacyWrapperBeforeRepository(t *testing.T) {
	repo := &fakeGroupMembershipProjectorRepository{}
	projector := &GroupMembershipProjector{repo: repo}

	err := projector.handle(context.Background(), []byte(`{"payload":{"event_id":"legacy"}}`))
	assert.Error(t, err)
	assert.True(t, kafka.IsPermanent(err))
	assert.Zero(t, repo.calls)
}
