# PoLE (Proof of Live Engagement)

[English](./README.md) | [中文](./README_CN.md)

A decentralized Layer 1 blockchain that transforms real player engagement in PC gaming into quantifiable on-chain value.

## Overview

PoLE is an innovative Layer 1 blockchain project that converts real player engagement in PC gaming ecosystems into quantifiable on-chain value. Through a distributed node network operated by players, PoLE captures game online data (using Steam's public API as the primary data source), calculates Game Value Score (GVS), and rewards participants with the native $POLE token.

## Architecture

```
PoLE/
├── core/                    # Core blockchain modules
│   ├── types/              # Core data types
│   └── consensus/           # PoS + PoLE consensus
├── data/                   # Data layer
│   ├── collector/          # Game data collection
│   └── availability/       # Data availability layer
├── execution/              # Execution layer
│   └── engine/             # GVS calculation engine
├── token/                 # Token economics
│   ├── economy/            # Mint, burn, supply
│   └── rewards/            # Reward distribution
├── governance/            # On-chain governance
└── network/               # P2P networking
    └── p2p/               # libp2p implementation
```

## Features

- **Hybrid Consensus**: PoS (Proof of Stake) + PoLE (Proof of Live Engagement)
- **GVS Calculation**: Game Value Score based on player engagement
- **Tier System**: Three-tier game classification (Tier 1/2/3)
- **Token Economics**: Inflation, burning mechanisms, reward distribution
- **On-chain Governance**: DAO-style proposal and voting
- **P2P Networking**: libp2p-based decentralized network

## Building

```bash
# Clone the repository
git clone https://github.com/pole-chain/pole.git
cd pole

# Build the node (Go 1.19+)
go build -o pole-node.exe ./cmd/node

# Run tests
go test ./...
```

## Running the node (Windows)

- **run.bat**: Testnet / vesting test, opens wallet.
- **run-mainnet.bat**: Mainnet node.
- **run-mainnet-mining.bat**: Mainnet + mining (Play-to-Earn auto-collect and rewards).
- CLI: `.\scripts\run.ps1 -Profile mainnet -Mining -OpenBrowser`. See [Testing](./docs_TESTING.md), [Mainnet launch](./docs_MAINNET_LAUNCH.md).

## Documentation

- [Doc index](./docs_INDEX.md) (by topic)
- [Whitepaper](./docs_PoLE_Whitepaper.md)
- [Technical Specification](./PoLE_Technical_Specification.md)
- [Testing](./docs_TESTING.md) · [Mainnet launch](./docs_MAINNET_LAUNCH.md)

## Roadmap

| Phase | Timeline | Description |
|-------|----------|-------------|
| Phase 0 | 2026 Q1 | Concept verification |
| Phase 1 | 2026 Q2-Q3 | Testnet launch |
| Phase 2 | 2026 Q4 | Mainnet Alpha |
| Phase 3 | 2027+ | Ecosystem expansion |

## Tokenomics

- **Total Supply**: 1 billion $POLE
- **Initial Inflation**: 20% annually
- **Token Distribution**:
  - Node Rewards: 60%
  - Ecosystem Fund: 20%
  - Community Incentives: 15%
  - Team: 5%

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting PRs.

## License

MIT License - see LICENSE file for details.
