package consts

const (
	CelestiaDockerPrefix = "tastora"
	// CleanupLabel is a docker label key targeted by DockerSetup when it cleans up docker resources.
	CleanupLabel = "tastora"
	// LabelPrefix is the reverse DNS format "namespace" for tastora Docker labels.
	LabelPrefix = "org.celestia.tastora."
	// NodeOwnerLabel indicates the logical node owning a particular object (probably a volume).
	NodeOwnerLabel = LabelPrefix + "node-owner"
	// UserRootString defines the default root user and group identifier in the format "UID:GID".
	UserRootString = "0:0"
	// FaucetAccountKeyName defines the default key name used for the faucet account.
	FaucetAccountKeyName = "faucet"
)
