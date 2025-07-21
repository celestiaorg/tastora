package rollkit

import (
	"bytes"
	"fmt"
	"net/http"
	"path/filepath"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

// Strategy implements ChainStrategy for Rollkit chains
type Strategy struct {
	AggregatorPassphrase string
}

func NewStrategy(aggregatorPassphrase string) *Strategy {
	return &Strategy{
		AggregatorPassphrase: aggregatorPassphrase,
	}
}

func (s *Strategy) GetPortMappings() nat.PortMap {
	return nat.PortMap{
		"2121/tcp":  {}, // p2p
		"7331/tcp":  {}, // rpc
		"9090/tcp":  {}, // grpc
		"1317/tcp":  {}, // api
		"26658/tcp": {}, // priv val
		"8080/tcp":  {}, // http
	}
}

func (s *Strategy) GetPortNameMapping() map[string]string {
	return map[string]string{
		"p2p":  "2121/tcp",
		"rpc":  "7331/tcp",
		"grpc": "9090/tcp",
		"api":  "1317/tcp",
		"http": "8080/tcp",
	}
}

func (s *Strategy) GetInitFlags(index int, homeDir string) []string {
	// rollkit-specific: only index 0 is aggregator
	if index != 0 {
		return []string{}
	}
	return []string{
		"--rollkit.node.aggregator",
		"--rollkit.signer.passphrase=" + s.AggregatorPassphrase,
		"--rollkit.signer.path=" + filepath.Join(homeDir, "config"),
	}
}

func (s *Strategy) GetStartFlags(index int, homeDir string) []string {
	return s.GetInitFlags(index, homeDir) // same logic for rollkit
}

func (s *Strategy) GetHealthEndpoint(hostRPCPort string) string {
	return fmt.Sprintf("http://%s/rollkit.v1.HealthService/Livez", hostRPCPort)
}

func (s *Strategy) IsNodeHealthy(client *http.Client, healthURL string, logger *zap.Logger) bool {
	req, err := http.NewRequest("POST", healthURL, bytes.NewBufferString("{}"))
	if err != nil {
		logger.Debug("failed to create health check request", zap.Error(err))
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.Debug("rollkit node not ready yet", zap.String("url", healthURL), zap.Error(err))
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true
	}

	logger.Debug("rollkit node not ready yet", zap.String("url", healthURL), zap.Int("status", resp.StatusCode))
	return false
}