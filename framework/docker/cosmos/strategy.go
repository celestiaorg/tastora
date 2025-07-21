package cosmos

import (
	"fmt"
	"net/http"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

// Strategy implements ChainStrategy for Cosmos SDK chains
type Strategy struct{}

func NewStrategy() *Strategy {
	return &Strategy{}
}

func (s *Strategy) GetPortMappings() nat.PortMap {
	return nat.PortMap{
		"26656/tcp": {}, // p2p
		"26657/tcp": {}, // rpc
		"9090/tcp":  {}, // grpc
		"1317/tcp":  {}, // api
		"1234/tcp":  {}, // priv val
	}
}

func (s *Strategy) GetPortNameMapping() map[string]string {
	return map[string]string{
		"p2p":  "26656/tcp",
		"rpc":  "26657/tcp",
		"grpc": "9090/tcp",
		"api":  "1317/tcp",
	}
}

func (s *Strategy) GetInitFlags(index int, homeDir string) []string {
	return []string{} // no extra flags for cosmos-sdk
}

func (s *Strategy) GetStartFlags(index int, homeDir string) []string {
	return []string{} // no extra flags for cosmos-sdk
}

func (s *Strategy) GetHealthEndpoint(hostRPCPort string) string {
	return fmt.Sprintf("http://%s/health", hostRPCPort)
}

func (s *Strategy) IsNodeHealthy(client *http.Client, healthURL string, logger *zap.Logger) bool {
	resp, err := client.Get(healthURL)
	if err != nil {
		logger.Debug("cosmos node not ready yet", zap.String("url", healthURL), zap.Error(err))
		return false
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == 200
}