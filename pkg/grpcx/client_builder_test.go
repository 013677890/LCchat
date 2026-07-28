package grpcx

import (
	"context"
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestBuildClientServiceConfig_ExactServiceAndMethod(t *testing.T) {
	cfg := DefaultClientRetryConfig(
		"/group.GroupService/GetGroupInfo",
		"/msg.MsgService/PullMessages",
	)
	cfg.Timeout = 2 * time.Second

	raw, err := buildClientServiceConfig(cfg, "round_robin")
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))

	// 负载均衡始终存在。
	lb, ok := payload["loadBalancingConfig"].([]any)
	require.True(t, ok)
	require.Len(t, lb, 1)

	methodConfigs, ok := payload["methodConfig"].([]any)
	require.True(t, ok)
	require.Len(t, methodConfigs, 1)

	mc := methodConfigs[0].(map[string]any)
	names := mc["name"].([]any)
	require.Len(t, names, 2)

	// 每条 name 必须同时带 service 与 method，禁止仅 service 的宽匹配。
	first := names[0].(map[string]any)
	assert.Equal(t, "group.GroupService", first["service"])
	assert.Equal(t, "GetGroupInfo", first["method"])
	second := names[1].(map[string]any)
	assert.Equal(t, "msg.MsgService", second["service"])
	assert.Equal(t, "PullMessages", second["method"])

	for _, name := range names {
		entry := name.(map[string]any)
		assert.Contains(t, entry, "service")
		assert.Contains(t, entry, "method")
		assert.NotEmpty(t, entry["service"])
		assert.NotEmpty(t, entry["method"])
	}

	retryPolicy := mc["retryPolicy"].(map[string]any)
	assert.EqualValues(t, 5, retryPolicy["maxAttempts"])
	assert.Equal(t, "2s", mc["timeout"])
}

func TestBuildClientServiceConfig_InvalidFullMethod(t *testing.T) {
	cases := []struct {
		name   string
		method string
	}{
		{name: "empty", method: ""},
		{name: "no_leading_slash", method: "group.GroupService/GetGroupInfo"},
		{name: "service_only", method: "/group.GroupService"},
		{name: "service_only_no_slash", method: "group.GroupService"},
		{name: "empty_service", method: "//GetGroupInfo"},
		{name: "empty_method", method: "/group.GroupService/"},
		{name: "extra_segment", method: "/group.GroupService/Get/Extra"},
		{name: "whitespace", method: "/group.GroupService/ GetGroupInfo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildClientServiceConfig(DefaultClientRetryConfig(tc.method), "round_robin")
			require.Error(t, err)
		})
	}
}

func TestBuildClientServiceConfig_DuplicateMethodRejected(t *testing.T) {
	_, err := buildClientServiceConfig(DefaultClientRetryConfig(
		"/group.GroupService/GetGroupInfo",
		"/group.GroupService/GetGroupInfo",
	), "round_robin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "重复")
}

func TestBuildClientServiceConfig_RetryNilKeepsLoadBalancing(t *testing.T) {
	raw, err := buildClientServiceConfig(nil, "")
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))

	lb, ok := payload["loadBalancingConfig"].([]any)
	require.True(t, ok)
	require.Len(t, lb, 1)
	_, hasMethodConfig := payload["methodConfig"]
	assert.False(t, hasMethodConfig)
}

func TestBuildClientServiceConfig_EmptyMethodsNoRetry(t *testing.T) {
	raw, err := buildClientServiceConfig(&ClientRetryConfig{Methods: nil}, "round_robin")
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	_, hasMethodConfig := payload["methodConfig"]
	assert.False(t, hasMethodConfig)
}

func TestBuildClientServiceConfig_ZeroTimeoutOmitted(t *testing.T) {
	cfg := DefaultClientRetryConfig("/auth.DeviceService/GetDeviceList")
	require.Equal(t, time.Duration(0), cfg.Timeout)

	raw, err := buildClientServiceConfig(cfg, "round_robin")
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	mc := payload["methodConfig"].([]any)[0].(map[string]any)
	_, hasTimeout := mc["timeout"]
	assert.False(t, hasTimeout, "Timeout=0 时不应写入 methodConfig.timeout，避免钳制方法级 deadline")
}

func TestParseRetryFullMethod(t *testing.T) {
	parsed, err := parseRetryFullMethod("/auth.DeviceService/GetDeviceList")
	require.NoError(t, err)
	assert.Equal(t, "/auth.DeviceService/GetDeviceList", parsed.FullMethod)
	assert.Equal(t, "auth.DeviceService", parsed.Service)
	assert.Equal(t, "GetDeviceList", parsed.Method)
}

// --- 集成：用 bufconn 验证方法级白名单重试行为 ---

// retryProbeServer 用 RegisterUnknownServiceHandler 挂载两个 full method，
// 便于在无 proto 生成桩的情况下验证 ServiceConfig 重试。
type retryProbeServer struct {
	readCalls  atomic.Int32
	writeCalls atomic.Int32
	// failReadFirstN 前 N 次读方法返回 Unavailable，之后成功。
	failReadFirstN int32
}

func (s *retryProbeServer) handle(ctx context.Context, req any) (any, error) {
	// 由外层 method 分发；这里不会直接调用。
	return &emptypb.Empty{}, nil
}

func startRetryProbeServer(t *testing.T, probe *retryProbeServer) (*grpc.ClientConn, func()) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	server := grpc.NewServer()

	// 使用自定义 service 注册：通过 UnknownServiceHandler 捕获 full method。
	// 更稳妥的做法是用 grpc.ServiceDesc 注册两个方法。
	serviceDesc := grpc.ServiceDesc{
		ServiceName: "probe.ProbeService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "ReadSomething",
				Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
					if err := dec(&emptypb.Empty{}); err != nil {
						return nil, err
					}
					handler := func(ctx context.Context, req any) (any, error) {
						n := probe.readCalls.Add(1)
						if n <= probe.failReadFirstN {
							return nil, status.Error(codes.Unavailable, "temporary")
						}
						return &emptypb.Empty{}, nil
					}
					if interceptor == nil {
						return handler(ctx, &emptypb.Empty{})
					}
					return interceptor(ctx, &emptypb.Empty{}, &grpc.UnaryServerInfo{
						Server:     srv,
						FullMethod: "/probe.ProbeService/ReadSomething",
					}, handler)
				},
			},
			{
				MethodName: "WriteSomething",
				Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
					if err := dec(&emptypb.Empty{}); err != nil {
						return nil, err
					}
					handler := func(ctx context.Context, req any) (any, error) {
						probe.writeCalls.Add(1)
						return nil, status.Error(codes.Unavailable, "temporary")
					}
					if interceptor == nil {
						return handler(ctx, &emptypb.Empty{})
					}
					return interceptor(ctx, &emptypb.Empty{}, &grpc.UnaryServerInfo{
						Server:     srv,
						FullMethod: "/probe.ProbeService/WriteSomething",
					}, handler)
				},
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "",
	}
	server.RegisterService(&serviceDesc, struct{}{})

	go func() {
		_ = server.Serve(lis)
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	// 仅对读方法配置重试；写方法不在白名单。
	retryCfg := DefaultClientRetryConfig("/probe.ProbeService/ReadSomething")
	retryCfg.MaxAttempts = 3
	retryCfg.InitialBackoff = 10 * time.Millisecond
	retryCfg.MaxBackoff = 50 * time.Millisecond
	// 集成测试缩短可重试码集合，避免干扰断言。
	retryCfg.RetryableStatusCodes = []string{"UNAVAILABLE"}

	serviceConfig, err := buildClientServiceConfig(retryCfg, "round_robin")
	require.NoError(t, err)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(serviceConfig),
	)
	require.NoError(t, err)

	cleanup := func() {
		_ = conn.Close()
		server.Stop()
		_ = lis.Close()
	}
	return conn, cleanup
}

func invokeProbe(ctx context.Context, conn *grpc.ClientConn, fullMethod string) error {
	return conn.Invoke(ctx, fullMethod, &emptypb.Empty{}, &emptypb.Empty{})
}

func TestClientRetry_WhitelistedReadRetriesOnUnavailable(t *testing.T) {
	probe := &retryProbeServer{failReadFirstN: 1}
	conn, cleanup := startRetryProbeServer(t, probe)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := invokeProbe(ctx, conn, "/probe.ProbeService/ReadSomething")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, probe.readCalls.Load(), int32(2), "白名单读方法在 Unavailable 后应至少重试一次")
}

func TestClientRetry_NonWhitelistedWriteNotRetried(t *testing.T) {
	probe := &retryProbeServer{}
	conn, cleanup := startRetryProbeServer(t, probe)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := invokeProbe(ctx, conn, "/probe.ProbeService/WriteSomething")
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, int32(1), probe.writeCalls.Load(), "非白名单写方法返回 Unavailable 后只能调用一次")
}

func TestClientRetry_ParentDeadlineTakesPriority(t *testing.T) {
	// 读方法永远返回 Unavailable，靠父 deadline 截断重试。
	probe := &retryProbeServer{failReadFirstN: 100}
	conn, cleanup := startRetryProbeServer(t, probe)
	defer cleanup()

	// 很短的父 deadline：应在预算内结束，且不会无限重试。
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := invokeProbe(ctx, conn, "/probe.ProbeService/ReadSomething")
	elapsed := time.Since(start)

	require.Error(t, err)
	// 可能是 DeadlineExceeded 或 Unavailable（最后一次 attempt 的结果）。
	code := status.Code(err)
	assert.True(t,
		code == codes.DeadlineExceeded || code == codes.Unavailable || code == codes.Canceled,
		"unexpected code: %v", code,
	)
	assert.Less(t, elapsed, 1500*time.Millisecond, "父 context deadline 应优先截断，不应拖到默认 MaxAttempts 全跑完")
}

func TestNewClient_RejectsInvalidRetryMethods(t *testing.T) {
	_, err := NewClient(ClientOptions{
		Address: "127.0.0.1:1",
		Retry:   DefaultClientRetryConfig("group.GroupService"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "full method")
}

func TestNewClient_RetryNilStillDials(t *testing.T) {
	// 地址不可达也没关系：NewClient 在 grpc-go 中是惰性连接，构造本身应成功。
	conn, err := NewClient(ClientOptions{
		Address: "127.0.0.1:1",
		Retry:   nil,
	})
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NoError(t, conn.Close())
}
