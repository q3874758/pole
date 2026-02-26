# OpenClaw 工作量证明 (Proof of Agent Work)

> 让 AI Agent 的工作变得可量化、可验证、可追溯。

## 🎯 愿景

解决 AI 工作透明性问题：
- AI 消耗了多少 token？
- AI 完成了多少任务？
- AI 创造了什么价值？
- 如何证明 AI 真的在工作？

## 🏗️ 架构

```
┌─────────────────────────────────────────────────────────────┐
│                    OpenClaw Agent                             │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ 日志监听    │  │ 事件解析    │  │ 工作量计算         │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                    本地追踪器 (Tracker)                       │
│  • 任务记录    • Token 统计    • 价值计算    • 证明生成  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    数据可视化                                 │
│  • 仪表盘    • 趋势图    • 任务分布    • 价值排名        │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    区块链层 (可选)                           │
│  • 工作证明存证    • 价值积分    • 奖励分发               │
└─────────────────────────────────────────────────────────────┘
```

## 📊 工作量指标

| 指标 | 权重 | 说明 |
|------|------|------|
| 代码生成 | 1.5x | 生成的代码行数 |
| Bug 修复 | 2.0x | 修复的问题数 |
| 部署运维 | 1.8x | 部署/运维任务 |
| 数据分析 | 1.4x | 分析任务 |
| 调研分析 | 1.3x | 调研任务 |
| 代码审查 | 1.2x | 审查任务 |
| 文字创作 | 1.0x | 写作任务 |
| 文档编写 | 0.8x | 文档任务 |

## 💰 价值公式

```
AVS = Σ(任务完成 × 权重) 
    + Σ(代码行 × 0.01) 
    + Σ(Bug修复 × 5) 
    + Σ(文字产出 × 0.001)
    - Σ(Token消耗 × 0.0001)
```

## 🚀 快速开始

### 1. 安装

```bash
git clone https://github.com/pole-chain/openclaw-work-proof.git
cd openclaw-work-proof
go build ./...
```

### 2. 启动追踪器

```bash
./bin/workproof-tracker --datadir ./data --port 18790
```

### 3. 集成 OpenClaw

```bash
# 启动 OpenClaw
openclaw gateway start

# 启动工作量追踪
./bin/workproof-integrator --openclaw-url http://localhost:18789
```

### 4. 查看仪表盘

```bash
# 启动仪表盘
cd dashboard
python -m http.server 8080

# 打开浏览器
# http://localhost:8080
```

## 📁 项目结构

```
openclaw-work-proof/
├── tracker/           # 工作量追踪器
│   └── tracker.go     # 核心追踪逻辑
├── integrator/        # OpenClaw 集成
│   └── openclaw.go   # OpenClaw 事件监听
├── contracts/        # 智能合约
│   └── WorkProof.sol # 区块链存证
├── dashboard/        # 可视化仪表盘
│   └── index.html   # Web 界面
├── cmd/              # 命令行工具
└── README.md
```

## 🔧 配置

### 追踪器配置

```json
{
  "data_dir": "./data",
  "api_port": 18790,
  "weights": {
    "coding": 1.5,
    "debug": 2.0,
    "deploy": 1.8,
    "writing": 1.0
  }
}
```

### OpenClaw 集成

```json
{
  "openclaw_url": "http://localhost:18789",
  "agent_id": "my-agent",
  "poll_interval": "10s"
}
```

## 🔐 工作证明

每个任务完成后生成唯一证明：

```json
{
  "id": "a1b2c3d4e5f6",
  "agent_id": "main-agent",
  "task_type": "coding",
  "tokens_used": 15000,
  "code_lines": 450,
  "bugs_fixed": 3,
  "value": 487.5,
  "proof_hash": "0x7f83b1657ff1fc53b92dc18148a1d65dfc2d4b1fa3d677284addd200126d9069",
  "timestamp": 1706236800
}
```

## ⛓️ 区块链存证 (可选)

部署智能合约到链上：

```bash
# 编译合约
solc contracts/WorkProof.sol --combined-json abi > build/WorkProof.abi.json
solc contracts/WorkProof.sol --bin > build/WorkProof.bin

# 部署 (需要 Remix 或 Hardhat)
```

## 📈 仪表盘预览

- 总任务数 / 完成任务数
- Token 消耗统计
- 代码行数 / Bug 修复数
- 任务类型分布饼图
- 价值趋势折线图
- 工作记录表格

## 🧪 测试

```bash
go test ./...
```

## 📜 许可证

MIT License

## 🤝 贡献

欢迎提交 Issue 和 PR！
