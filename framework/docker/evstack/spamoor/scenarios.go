package spamoor

// Scenario name constants matching Spamoor's CLI identifiers.
// Use these to avoid typos when creating spammers via the API.
const (
	ScenarioEOATX            = "eoatx"
	ScenarioERC20TX          = "erc20tx"
	ScenarioERC721TX         = "erc721tx"
	ScenarioERC1155TX        = "erc1155tx"
	ScenarioCallTX           = "calltx"
	ScenarioDeployTX         = "deploytx"
	ScenarioDeployDestruct   = "deploy-destruct"
	ScenarioSetCodeTX        = "setcodetx"
	ScenarioUniswapSwaps     = "uniswap-swaps"
	ScenarioBlobs            = "blobs"
	ScenarioBlobAverage      = "blob-average"
	ScenarioBlobReplacements = "blob-replacements"
	ScenarioBlobConflicting  = "blob-conflicting"
	ScenarioBlobCombined     = "blob-combined"
	ScenarioGasBurnerTX      = "gasburnertx"
	ScenarioStorageSpam      = "storagespam"
	ScenarioGeasTX           = "geastx"
	ScenarioXenToken         = "xentoken"
	ScenarioTaskRunner       = "taskrunner"
)
