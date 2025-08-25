package dataavailability

import "github.com/celestiaorg/tastora/framework/testutil/toml"

type StartOption func(*StartOptions)

// StartOptions represents the configuration options required for starting a DA node.
type StartOptions struct {
	// ChainID is the chain ID.
	ChainID string
	// StartArguments specifies any additional start arguments after "celestia start <type>"
	StartArguments []string
	// EnvironmentVariables specifies any environment variables that should be passed to the Node
	// e.g. the CELESTIA_CUSTOM environment variable.
	EnvironmentVariables map[string]string
	// ConfigModifications specifies modifications to be applied to config files.
	// The map key is the file path, and the value is the TOML modifications to apply.
	ConfigModifications map[string]toml.Toml
}

// WithAdditionalStartArguments sets the additional start arguments to be used.
func WithAdditionalStartArguments(startArgs ...string) StartOption {
	return func(o *StartOptions) {
		o.StartArguments = startArgs
	}
}

// WithEnvironmentVariables sets the environment variables to be used.
func WithEnvironmentVariables(envVars map[string]string) StartOption {
	return func(o *StartOptions) {
		o.EnvironmentVariables = envVars
	}
}

// WithChainID sets the chainID.
func WithChainID(chainID string) StartOption {
	return func(o *StartOptions) {
		o.ChainID = chainID
	}
}

// WithConfigModifications sets the config modifications to be applied to config files.
func WithConfigModifications(configModifications map[string]toml.Toml) StartOption {
	return func(o *StartOptions) {
		o.ConfigModifications = configModifications
	}
}