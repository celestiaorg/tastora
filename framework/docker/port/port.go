package port

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

var mu sync.RWMutex

type Listeners []net.Listener

func (l Listeners) CloseAll() {
	for _, listener := range l {
		_ = listener.Close()
	}
}

// OpenListener opens a listener on a port. Set to 0 to get a random port.
func OpenListener(port int) (*net.TCPListener, error) {
	mu.Lock()
	defer mu.Unlock()

	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return l, nil
}

// GenerateBindings will find open ports on the local
// host and bind them to the container ports.
// This is useful for cases where you want to find a random open port.
func GenerateBindings(pairs network.PortMap) (network.PortMap, Listeners, error) {
	listeners := make(Listeners, 0)
	bindings := make(network.PortMap)

	for port := range pairs {
		// Listen on a random port
		listener, err := OpenListener(0)
		if err != nil {
			listeners.CloseAll()
			return nil, nil, err
		}
		listeners = append(listeners, listener)

		// Extract the port from the listener
		parts := strings.Split(listener.Addr().String(), ":")
		if len(parts) < 2 {
			listeners.CloseAll()
			return nil, nil, fmt.Errorf("failed to parse address: %s", listener.Addr().String())
		}
		portStr := parts[len(parts)-1]

		bindings[port] = []network.PortBinding{
			{
				HostIP:   netip.MustParseAddr("127.0.0.1"),
				HostPort: portStr,
			},
		}
	}

	return bindings, listeners, nil
}

// GetForHost returns a resource's published port with an address.
func GetForHost(cont container.InspectResponse, portID string) string {
	if cont.NetworkSettings == nil {
		return ""
	}

	ports := cont.NetworkSettings.Ports
	if len(ports) == 0 {
		return ""
	}

	p, err := network.ParsePort(portID)
	if err != nil {
		return ""
	}
	if bindings, exists := ports[p]; exists && len(bindings) > 0 {
		return bindings[0].HostIP.String() + ":" + bindings[0].HostPort
	}

	return ""
}
