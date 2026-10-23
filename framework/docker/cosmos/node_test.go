package cosmos

import (
	"context"
	"net"
	"testing"
	"time"

	rpcclient "github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type readyRPCClient struct{ rpcclient.Client }

func (readyRPCClient) Status(context.Context) (*coretypes.ResultStatus, error) {
	return &coretypes.ResultStatus{}, nil
}

func TestReadinessWaitsForSDKGRPC(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	node := &ChainNode{Client: readyRPCClient{}, GrpcConn: conn}

	// Comet responds, but the SDK has not started serving yet.
	require.ErrorContains(t, node.checkReady(context.Background(), true), "SDK gRPC not ready")
	server := grpc.NewServer()
	t.Cleanup(server.Stop)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	require.Eventually(t, func() bool {
		return node.checkReady(context.Background(), true) == nil
	}, 5*time.Second, 10*time.Millisecond)
	server.Stop()
	require.NoError(t, <-serveErr)
}

func TestSDKGRPCEnabled(t *testing.T) {
	for _, test := range []struct {
		name    string
		config  string
		args    []string
		enabled bool
	}{
		{name: "default", enabled: true},
		{name: "config enabled", config: "[grpc]\nenable = true", enabled: true},
		{name: "config disabled", config: "[grpc]\nenable = false"},
		{name: "flag disables", config: "[grpc]\nenable = true", args: []string{"--grpc.enable=false"}},
		{name: "flag enables", config: "[grpc]\nenable = false", args: []string{"--grpc.enable=true"}, enabled: true},
		{name: "bare flag enables", config: "[grpc]\nenable = false", args: []string{"--grpc.enable"}, enabled: true},
		{name: "last flag wins", args: []string{"--grpc.enable=true", "--grpc.enable=false"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			enabled, err := sdkGRPCEnabled([]byte(test.config), test.args)
			require.NoError(t, err)
			require.Equal(t, test.enabled, enabled)
			if !enabled {
				node := &ChainNode{Client: readyRPCClient{}}
				require.NoError(t, node.checkReady(context.Background(), enabled))
			}
		})
	}
	_, err := sdkGRPCEnabled([]byte("[broken"), nil)
	require.ErrorContains(t, err, "parse SDK readiness config")
	_, err = sdkGRPCEnabled(nil, []string{"--grpc.enable=invalid"})
	require.ErrorContains(t, err, "parse --grpc.enable")
}
