package grpcx

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBreakerIsSuccessful(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool // true = 视为成功，不计熔断失败
	}{
		{"nil", nil, true},
		{"context canceled", context.Canceled, true},
		{"grpc canceled", status.Error(codes.Canceled, "client gone"), true},

		// 业务错误：不应触发熔断
		{"invalid argument", status.Error(codes.InvalidArgument, "bad param"), true},
		{"unauthenticated (wrong password)", status.Error(codes.Unauthenticated, "password error"), true},
		{"permission denied", status.Error(codes.PermissionDenied, "no perm"), true},
		{"not found", status.Error(codes.NotFound, "user not found"), true},
		{"already exists", status.Error(codes.AlreadyExists, "dup"), true},
		{"failed precondition", status.Error(codes.FailedPrecondition, "precond"), true},
		{"resource exhausted (业务限流)", status.Error(codes.ResourceExhausted, "too frequent"), true},

		// 基础设施故障：应计入失败率
		{"unavailable", status.Error(codes.Unavailable, "conn refused"), false},
		{"deadline exceeded", status.Error(codes.DeadlineExceeded, "slow"), false},
		{"internal", status.Error(codes.Internal, "boom"), false},
		{"unknown", status.Error(codes.Unknown, "?"), false},
		{"data loss", status.Error(codes.DataLoss, "lost"), false},

		// 非 gRPC status 错误：保守计为失败
		{"plain error", errors.New("some non-grpc error"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BreakerIsSuccessful(tc.err); got != tc.want {
				t.Fatalf("BreakerIsSuccessful(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
