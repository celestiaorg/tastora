package hyperlane

import "github.com/celestiaorg/tastora/framework/types"

var (
    DeployerNodeType = hyperlaneNodeType("hyperlane-deployer")
    AgentNodeType    = hyperlaneNodeType("hyperlane-agent")
)

type hyperlaneNodeType string

func (t hyperlaneNodeType) String() string { return string(t) }

var _ types.NodeType = hyperlaneNodeType("")
