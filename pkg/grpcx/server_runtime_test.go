package grpcx

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestServeReturnsNilAfterGracefulStop(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(server, listener)
	}()

	require.NoError(t, GracefulStop(context.Background(), server, time.Second))

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("grpc Serve did not return after graceful stop")
	}
}
