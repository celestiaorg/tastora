package relayer

// This file contains types that represent the structure of Hermes relayer JSON output.
// These types are used to parse responses from Hermes commands when using the --json flag.

// ChannelCreationResponse represents the response from hermes create channel command
type ChannelCreationResponse struct {
	Result CreateChannelResult `json:"result"`
}

// CreateChannelResult holds channel information for both sides
type CreateChannelResult struct {
	ASide ChannelSide `json:"a_side"`
	BSide ChannelSide `json:"b_side"`
}

// ChannelSide captures the channel ID for each side
type ChannelSide struct {
	ChannelID string `json:"channel_id"`
}

// ConnectionCreationResponse represents the response from hermes create connection command
type ConnectionCreationResponse struct {
	Result CreateConnectionResult `json:"result"`
}

// CreateConnectionResult holds connection information for both sides
type CreateConnectionResult struct {
	ASide ConnectionSide `json:"a_side"`
	BSide ConnectionSide `json:"b_side"`
}

// ConnectionSide captures the connection ID for each side
type ConnectionSide struct {
	ConnectionID string `json:"connection_id"`
}