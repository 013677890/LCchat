package apperr

import (
	"errors"
	"testing"

	"github.com/013677890/LCchat-Backend/consts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestErrorImplementsGRPCStatus 验证裸 *apperr.Error 被 gRPC 框架识别后
// 映射为正确的传输层 code，而非兜底 Unknown，并在 details 中携带业务码。
func TestErrorImplementsGRPCStatus(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantGRPC codes.Code
		wantBiz  int
	}{
		{"param", New(consts.CodeParamError), codes.InvalidArgument, consts.CodeParamError},
		{"timeout", New(consts.CodeTimeoutError), codes.DeadlineExceeded, consts.CodeTimeoutError},
		{"permission", New(consts.CodePermissionDeny), codes.PermissionDenied, consts.CodePermissionDeny},
		{"internal", New(consts.CodeInternalError), codes.Internal, consts.CodeInternalError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟 gRPC 框架对 handler/拦截器返回错误的处理路径。
			st, ok := status.FromError(tc.err)
			require.True(t, ok, "framework 应能从裸 apperr 解析出 status")
			assert.Equal(t, tc.wantGRPC, st.Code())

			// 还原业务码应与原始码一致。
			restored := FromStatus(st.Err())
			assert.Equal(t, tc.wantBiz, Code(restored))
		})
	}
}

// TestGRPCStatusSanitizesMessage 验证 GRPCStatus 不泄漏内部 cause 文案
// （如 PGV 校验原文），对外只暴露业务码标准文案。
func TestGRPCStatusSanitizesMessage(t *testing.T) {
	raw := "invalid Email: value must be a valid email address"
	err := Wrap(errors.New(raw), consts.CodeParamError, raw)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, consts.GetMessage(consts.CodeParamError), st.Message())
	assert.NotContains(t, st.Message(), "Email")
}

// TestFromStatusKeepsLocalAppError 验证本地 apperr 经 FromStatus 仍按原对象返回，
// 保留堆栈与已记录日志标记（GRPCStatus 实现后这层前置判断不可丢）。
func TestFromStatusKeepsLocalAppError(t *testing.T) {
	original := Wrap(errors.New("db down"), consts.CodeInternalError, "内部错误")
	MarkLogged(original)

	got := FromStatus(original)
	assert.True(t, HasStack(got), "应保留堆栈")
	assert.True(t, IsLogged(got), "应保留已记录日志标记")
	assert.Equal(t, consts.CodeInternalError, Code(got))
}

// TestFromStatusRejectsLegacyMessageNumericBizCode 验证不再把 status message 纯数字当业务码。
func TestFromStatusRejectsLegacyMessageNumericBizCode(t *testing.T) {
	// 旧协议：status.Error(codes.Unknown, "11001") 曾被解析为业务码 11001。
	legacy := status.Error(codes.Unknown, "11001")
	got := FromStatus(legacy)
	assert.Equal(t, consts.CodeInternalError, Code(got), "纯数字 message 不得再当作业务码")
	assert.NotEqual(t, 11001, Code(got))
}

// TestFromStatusMapsTransportCodeWithoutDetails 验证无 biz_code 时仅按传输层码粗映射。
func TestFromStatusMapsTransportCodeWithoutDetails(t *testing.T) {
	got := FromStatus(status.Error(codes.Unavailable, "connection refused"))
	assert.Equal(t, consts.CodeServiceUnavailable, Code(got))

	got = FromStatus(status.Error(codes.DeadlineExceeded, "deadline"))
	assert.Equal(t, consts.CodeTimeoutError, Code(got))
}
