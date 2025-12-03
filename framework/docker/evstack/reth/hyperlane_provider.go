package reth

import (
    "context"
    "github.com/celestiaorg/tastora/framework/docker/hyperlane"
)

// Ensure Node satisfies the new interface.
// var _ hyperlane.ChainConfigProvider = (*Node)(nil)

func (n *Node) GetHyperlaneRegistryMetadata(ctx context.Context) (hyperlane.ChainMetadata, error) {
    return n.GetHyperlaneChainMetadata(ctx)
}

func (n *Node) GetHyperlaneRelayerChainConfig(ctx context.Context) (hyperlane.RelayerChainConfig, error) {
    meta, err := n.GetHyperlaneChainMetadata(ctx)
    if err != nil {
        return hyperlane.RelayerChainConfig{}, err
    }

    cfg := hyperlane.RelayerChainConfig{
        Name:        meta.Name,
        ChainID:     meta.ChainID,
        DomainID:    meta.DomainID,
        DisplayName: meta.DisplayName,
        Protocol:    meta.Protocol,
        IsTestnet:   meta.IsTestnet,
        NativeToken: &meta.NativeToken,
        Blocks:      meta.Blocks,
        RpcURLs:     meta.RpcURLs,
    }

    // signer
    signer := &hyperlane.SignerConfig{Key: meta.SignerKey}
    if meta.Protocol == "ethereum" {
        signer.Type = "hexKey"
    } else if meta.Protocol == "cosmosnative" {
        signer.Type = "cosmosKey"
        signer.Prefix = meta.Bech32Prefix
    }
    cfg.Signer = signer

    // protocol-specific fields
    if meta.Protocol == "ethereum" && meta.CoreContracts != nil {
        cfg.Mailbox = meta.CoreContracts.Mailbox
        cfg.InterchainSecurityModule = meta.CoreContracts.InterchainSecurityModule
        cfg.InterchainGasPaymaster = meta.CoreContracts.InterchainGasPaymaster
        cfg.MerkleTreeHook = meta.CoreContracts.MerkleTreeHook
        cfg.ProxyAdmin = meta.CoreContracts.ProxyAdmin
        cfg.ValidatorAnnounce = meta.CoreContracts.ValidatorAnnounce
        cfg.AggregationHook = meta.CoreContracts.AggregationHook
        cfg.DomainRoutingIsm = meta.CoreContracts.DomainRoutingIsm
        cfg.FallbackRoutingHook = meta.CoreContracts.FallbackRoutingHook
        cfg.ProtocolFee = meta.CoreContracts.ProtocolFee
        cfg.StorageGasOracle = meta.CoreContracts.StorageGasOracle
        cfg.TestRecipient = meta.CoreContracts.TestRecipient
    }

    if meta.Protocol == "cosmosnative" {
        cfg.Bech32Prefix = meta.Bech32Prefix
        cfg.CanonicalAsset = meta.CanonicalAsset
        cfg.ContractAddressBytes = meta.ContractAddressBytes
        cfg.RestURLs = meta.RestURLs
        cfg.GrpcURLs = meta.GrpcURLs
        cfg.GasPrice = meta.GasPrice
        cfg.Index = meta.IndexConfig
        cfg.Slip44 = meta.Slip44
    }

    return cfg, nil
}

