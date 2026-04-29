package grpcx

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestInternalCallerInterceptor(t *testing.T) {
	whitelist := map[string][]string{
		"/auth.InternalAuthService/FindAccountByEmail": {"gateway", "relation-service"},
		"/auth.InternalAuthService/UpdateLoginDisplay": {"user-service"},
	}
	interceptor := InternalCallerInterceptor(whitelist)
	dummyHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	tests := []struct {
		name       string
		fullMethod string
		caller     string // empty = no header
		wantCode   codes.Code
		wantOK     bool
	}{
		{
			name:       "internal method with valid caller",
			fullMethod: "/auth.InternalAuthService/FindAccountByEmail",
			caller:     "gateway",
			wantOK:     true,
		},
		{
			name:       "internal method with another valid caller",
			fullMethod: "/auth.InternalAuthService/FindAccountByEmail",
			caller:     "relation-service",
			wantOK:     true,
		},
		{
			name:       "internal method with unauthorized caller",
			fullMethod: "/auth.InternalAuthService/FindAccountByEmail",
			caller:     "evil-service",
			wantCode:   codes.PermissionDenied,
		},
		{
			name:       "internal method without caller header",
			fullMethod: "/auth.InternalAuthService/FindAccountByEmail",
			caller:     "",
			wantCode:   codes.PermissionDenied,
		},
		{
			name:       "external method without caller header (ok)",
			fullMethod: "/auth.AuthService/Login",
			caller:     "",
			wantOK:     true,
		},
		{
			name:       "external method with caller header (rejected)",
			fullMethod: "/auth.AuthService/Login",
			caller:     "gateway",
			wantCode:   codes.PermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.caller != "" {
				md := metadata.Pairs(MetadataInternalCaller, tt.caller)
				ctx = metadata.NewIncomingContext(ctx, md)
			}

			info := &grpc.UnaryServerInfo{FullMethod: tt.fullMethod}
			resp, err := interceptor(ctx, nil, info, dummyHandler)

			if tt.wantOK {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if resp != "ok" {
					t.Fatalf("expected resp='ok', got %v", resp)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Fatalf("expected gRPC status error, got: %v", err)
				}
				if st.Code() != tt.wantCode {
					t.Fatalf("expected code %v, got %v: %s", tt.wantCode, st.Code(), st.Message())
				}
			}
		})
	}
}
