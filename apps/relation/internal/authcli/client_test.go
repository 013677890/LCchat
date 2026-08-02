package authcli

import (
	"context"
	"errors"
	"testing"

	authpb "github.com/013677890/LCchat-Backend/apps/auth/pb"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeInternalAuthClient 仅用于驱动 BatchCheckAccountStatus 分支，其余方法不应被调用。
type fakeInternalAuthClient struct {
	batchFn func(context.Context, *authpb.BatchCheckAccountStatusRequest, ...grpc.CallOption) (*authpb.BatchCheckAccountStatusResponse, error)
}

func (f *fakeInternalAuthClient) FindAccountByEmail(ctx context.Context, in *authpb.FindAccountByEmailRequest, opts ...grpc.CallOption) (*authpb.FindAccountByEmailResponse, error) {
	panic("不应调用 FindAccountByEmail")
}

func (f *fakeInternalAuthClient) FindAccountByTelephone(ctx context.Context, in *authpb.FindAccountByTelephoneRequest, opts ...grpc.CallOption) (*authpb.FindAccountByTelephoneResponse, error) {
	panic("不应调用 FindAccountByTelephone")
}

func (f *fakeInternalAuthClient) UpdateLoginDisplay(ctx context.Context, in *authpb.UpdateLoginDisplayRequest, opts ...grpc.CallOption) (*authpb.UpdateLoginDisplayResponse, error) {
	panic("不应调用 UpdateLoginDisplay")
}

func (f *fakeInternalAuthClient) BatchCheckAccountStatus(ctx context.Context, in *authpb.BatchCheckAccountStatusRequest, opts ...grpc.CallOption) (*authpb.BatchCheckAccountStatusResponse, error) {
	return f.batchFn(ctx, in, opts...)
}

func TestIsAccountVisible(t *testing.T) {
	tests := []struct {
		name        string
		items       []*authpb.AccountStatusItem
		rpcErr      error
		wantVisible bool
		wantErrCode int
	}{
		{
			name:        "exists_and_normal",
			items:       []*authpb.AccountStatusItem{{UserUuid: "u2", Exists: true, Status: 0}},
			wantVisible: true,
		},
		{
			name:  "exists_but_deregistered",
			items: []*authpb.AccountStatusItem{{UserUuid: "u2", Exists: true, Status: 1}},
		},
		{
			name:  "not_exists",
			items: []*authpb.AccountStatusItem{{UserUuid: "u2", Exists: false}},
		},
		{
			name:  "item_missing",
			items: []*authpb.AccountStatusItem{{UserUuid: "other", Exists: true, Status: 0}},
		},
		{
			name:        "transport_error_wrapped_internal",
			rpcErr:      errors.New("connection refused"),
			wantErrCode: consts.CodeInternalError,
		},
		{
			name:        "business_error_passthrough",
			rpcErr:      apperr.ToStatus(apperr.New(consts.CodeParamError)),
			wantErrCode: consts.CodeParamError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{internalAuthClient: &fakeInternalAuthClient{
				batchFn: func(_ context.Context, in *authpb.BatchCheckAccountStatusRequest, _ ...grpc.CallOption) (*authpb.BatchCheckAccountStatusResponse, error) {
					require.Equal(t, []string{"u2"}, in.GetUserUuids())
					if tt.rpcErr != nil {
						return nil, tt.rpcErr
					}
					return &authpb.BatchCheckAccountStatusResponse{Items: tt.items}, nil
				},
			}}

			visible, err := client.IsAccountVisible(context.Background(), "u2")

			if tt.wantErrCode != 0 {
				require.Error(t, err)
				assert.Equal(t, tt.wantErrCode, apperr.Code(err))
				assert.False(t, visible)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantVisible, visible)
		})
	}

	t.Run("empty_uuid", func(t *testing.T) {
		client := &Client{internalAuthClient: &fakeInternalAuthClient{}}
		visible, err := client.IsAccountVisible(context.Background(), "")
		require.Error(t, err)
		assert.Equal(t, consts.CodeParamError, apperr.Code(err))
		assert.False(t, visible)
	})

	t.Run("nil_client", func(t *testing.T) {
		visible, err := NewClient(nil).IsAccountVisible(context.Background(), "u2")
		require.Error(t, err)
		assert.Equal(t, consts.CodeInternalError, apperr.Code(err))
		assert.False(t, visible)
	})
}
