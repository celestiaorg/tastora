package internal

import sdk "github.com/cosmos/cosmos-sdk/types"

// TemporarilyModifySDKConfigPrefix updates the global config to use the provided prefix.
// A function is returned which reverts the change.
// This operation is required because the sdk utilizies a global config, and if dealing with addresses with different
// prefixes, the state of the config much be updated accordingly.
func TemporarilyModifySDKConfigPrefix(bech32prefix string) func() {
	// Save current global bech32 config
	config := sdk.GetConfig()
	currentAccountPrefix := config.GetBech32AccountAddrPrefix()
	currentAccountPubPrefix := config.GetBech32AccountPubPrefix()
	currentValidatorPrefix := config.GetBech32ValidatorAddrPrefix()
	currentValidatorPubPrefix := config.GetBech32ValidatorPubPrefix()
	currentConsensusPrefix := config.GetBech32ConsensusAddrPrefix()
	currentConsensusPubPrefix := config.GetBech32ConsensusPubPrefix()

	// set the global bech32 config for this chain
	config.SetBech32PrefixForAccount(bech32prefix, bech32prefix+"pub")
	config.SetBech32PrefixForValidator(bech32prefix+"valoper", bech32prefix+"valoperpub")
	config.SetBech32PrefixForConsensusNode(bech32prefix+"valcons", bech32prefix+"valconspub")

	// return a function that reverts the changes we just made.
	return func() {
		config.SetBech32PrefixForAccount(currentAccountPrefix, currentAccountPubPrefix)
		config.SetBech32PrefixForValidator(currentValidatorPrefix, currentValidatorPubPrefix)
		config.SetBech32PrefixForConsensusNode(currentConsensusPrefix, currentConsensusPubPrefix)
	}
}
