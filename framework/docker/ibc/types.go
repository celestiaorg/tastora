package ibc

// CreateChannelOptions defines options for creating an IBC channel.
type CreateChannelOptions struct {
	SourcePortName string
	DestPortName   string
	Order          ChannelOrder
	Version        string
}

// ChannelOrder represents the ordering of an IBC channel.
type ChannelOrder string

const (
	OrderOrdered   ChannelOrder = "ordered"
	OrderUnordered ChannelOrder = "unordered"
)

// Channel represents an IBC channel between two chains.
type Channel struct {
	ChannelID        string
	CounterpartyID   string
	PortID           string
	CounterpartyPort string
	State            string
	Order            ChannelOrder
	Version          string
}

// Connection represents an IBC connection between two chains.
type Connection struct {
	ConnectionID         string
	CounterpartyID       string
	ClientID             string
	CounterpartyClientID string
	State                string
}
