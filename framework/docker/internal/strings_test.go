package internal

import (
	"reflect"
	"testing"
)

func TestParseCommandLineArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{
			name:     "empty args",
			args:     []string{},
			expected: map[string]string{},
		},
		{
			name:     "single key-value pair",
			args:     []string{"--rpc.laddr=tcp://0.0.0.0:26757"},
			expected: map[string]string{"rpc.laddr": "tcp://0.0.0.0:26757"},
		},
		{
			name:     "multiple key-value pairs",
			args:     []string{"--rpc.laddr=tcp://0.0.0.0:26757", "--grpc.address=0.0.0.0:9090"},
			expected: map[string]string{"rpc.laddr": "tcp://0.0.0.0:26757", "grpc.address": "0.0.0.0:9090"},
		},
		{
			name:     "flag without value",
			args:     []string{"--force-no-bbr"},
			expected: map[string]string{"force-no-bbr": ""},
		},
		{
			name:     "mixed flags and key-value pairs",
			args:     []string{"--rpc.laddr=tcp://0.0.0.0:26757", "--force-no-bbr", "--timeout-commit=1s"},
			expected: map[string]string{"rpc.laddr": "tcp://0.0.0.0:26757", "force-no-bbr": "", "timeout-commit": "1s"},
		},
		{
			name:     "args with dash are parsed",
			args:     []string{"-rpc.laddr=tcp://0.0.0.0:26757", "start", "--force-no-bbr"},
			expected: map[string]string{"rpc.laddr": "tcp://0.0.0.0:26757", "force-no-bbr": ""},
		},
		{
			name:     "value with equals sign",
			args:     []string{"--config=key=value"},
			expected: map[string]string{"config": "key=value"},
		},
		{
			name:     "empty value",
			args:     []string{"--rpc.laddr="},
			expected: map[string]string{"rpc.laddr": ""},
		},
		{
			name:     "complex values",
			args:     []string{"--minimum-gas-prices=0.025utia", "--rpc.grpc_laddr=tcp://0.0.0.0:9098"},
			expected: map[string]string{"minimum-gas-prices": "0.025utia", "rpc.grpc_laddr": "tcp://0.0.0.0:9098"},
		},
		{
			name:     "all port configurations",
			args:     []string{"--rpc.laddr=tcp://0.0.0.0:26757", "--grpc.address=0.0.0.0:9091", "--api.address=tcp://0.0.0.0:1318", "--p2p.laddr=tcp://0.0.0.0:26658"},
			expected: map[string]string{"rpc.laddr": "tcp://0.0.0.0:26757", "grpc.address": "0.0.0.0:9091", "api.address": "tcp://0.0.0.0:1318", "p2p.laddr": "tcp://0.0.0.0:26658"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCommandLineArgs(tt.args)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseCommandLineArgs() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
