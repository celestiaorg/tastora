package ibc

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