package docker

import (
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/stretchr/testify/require"
)

func TestGovernanceVoteOnProposal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	testCfg := setupDockerTest(t)

	chain, err := testCfg.ChainBuilder.
		WithNodes(
			cosmos.NewChainNodeConfigBuilder().Build(),
			cosmos.NewChainNodeConfigBuilder().Build(),
			cosmos.NewChainNodeConfigBuilder().Build(),
		).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	ctx := testCfg.Ctx

	govModuleAddress := authtypes.NewModuleAddress("gov").String()

	updateParamsMsg := &banktypes.MsgUpdateParams{
		Authority: govModuleAddress,
		Params:    banktypes.DefaultParams(),
	}

	proposal := createGovProposal(t, chain, updateParamsMsg)

	submittedProposal, err := chain.SubmitAndVoteOnGovV1Proposal(ctx, proposal, govv1.VoteOption_VOTE_OPTION_YES)
	require.NoError(t, err)
	require.NotNil(t, submittedProposal, "proposal should be returned")

	require.Equal(t, govv1.ProposalStatus_PROPOSAL_STATUS_PASSED, submittedProposal.Status,
		"proposal should have passed with all validators voting yes")

	t.Logf("Proposal %d passed successfully", submittedProposal.Id)
}

// createGovProposal creates a governance proposal with the provided messages and a standard deposit.
func createGovProposal(t *testing.T, chain *cosmos.Chain, msgs ...sdk.Msg) *govv1.MsgSubmitProposal {
	t.Helper()

	anyMsgs := make([]*codectypes.Any, len(msgs))
	for i, msg := range msgs {
		anyMsg, err := codectypes.NewAnyWithValue(msg)
		require.NoError(t, err)
		anyMsgs[i] = anyMsg
	}

	depositAmount := sdk.NewCoins(sdk.NewCoin(chain.Config.Denom, sdkmath.NewInt(1000)))

	proposal := &govv1.MsgSubmitProposal{
		Messages:       anyMsgs,
		InitialDeposit: depositAmount,
		Proposer:       chain.GetFaucetWallet().GetFormattedAddress(),
		Title:          "Test Proposal",
		Summary:        "Test proposal for governance voting",
	}

	return proposal
}
