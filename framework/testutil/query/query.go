package query

import (
	"context"
	sdkmath "cosmossdk.io/math"
	"fmt"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/gogoproto/grpc"
)

// Balance queries the balance of an address for a specific denom.
func Balance(ctx context.Context, grpcConn grpc.ClientConn, address string, denom string) (sdkmath.Int, error) {
	bankClient := banktypes.NewQueryClient(grpcConn)

	balanceReq := &banktypes.QueryBalanceRequest{
		Address: address,
		Denom:   denom,
	}

	resp, err := bankClient.Balance(ctx, balanceReq)
	if err != nil {
		return sdkmath.ZeroInt(), fmt.Errorf("failed to query balance for %s denom %s: %w", address, denom, err)
	}

	if resp.Balance == nil {
		return sdkmath.ZeroInt(), nil
	}

	return resp.Balance.Amount, nil
}
