# PoLE 项目优化建议

> 生成时间: 2026-02-25

## 🔴 严重问题

### 1. 依赖混乱
- `go.mod` 用 Go 1.24，但 `Cargo.toml` 用 Rust 2021 edition
- Go 项目依赖太少（只有4个），缺少常用库

### 2. 代码未完成
- `tendermint.go` 引用了 Tendermint 但没实现
- `tls.go` 存在但可能未完成

---

## 🟡 技术优化

### 1. 共识模块
- 当前是简化版 Tendermint，建议换成 **Cosmos SDK** 或 **CometBFT**
- 缺少签名验证、轮次同步等关键逻辑

### 2. 数据采集
- Steam API 容易受限，考虑：
  - 缓存层
  - 多个数据源冗余 (SteamDB, PlayerIGN)
  - 去中心化预言机 (Chainlink/-band)

### 3. 安全
- `signer.go` 用了基础 sha256，建议用 **ed25519** 或 **secp256k1**
- 缺少签名验证、交易验证

### 4. 网络层
- libp2p 代码是占位符，需要完整实现节点发现和 gossipsub

---

## 🟢 架构建议

| 模块 | 当前 | 建议 |
|------|------|------|
| SDK | 从零开始 | Cosmos SDK |
| 共识 | 手写 Tendermint | CometBFT |
| 合约 | 无 | Ethermint 兼容 EVM |
| 存储 | 未实现 | IAVL 树 |
