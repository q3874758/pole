#!/bin/bash
# PoLE 主网启动脚本

set -e

# 配置
CHAIN_ID="pole-mainnet-1"
DATA_DIR="${POLE_DATA_DIR:-data/mainnet}"
GENESIS_FILE="${POLE_GENESIS_FILE:-config/mainnet/genesis.json}"
RPC_PORT="${POLE_RPC_PORT:-9090}"
P2P_PORT="${POLE_P2P_PORT:-26656}"
PROMETHEUS_PORT="${POLE_PROMETHEUS_PORT:-9091}"
LOG_LEVEL="${POLE_LOG_LEVEL:-info}"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  PoLE Mainnet Node${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Chain ID: $CHAIN_ID"
echo "Data Dir: $DATA_DIR"
echo "Genesis:  $GENESIS_FILE"
echo ""

# 检查创世文件
if [ ! -f "$GENESIS_FILE" ]; then
    echo -e "${RED}错误: 创世文件不存在: $GENESIS_FILE${NC}"
    echo "请先创建创世文件: go run cmd/genesis/main.go"
    exit 1
fi

# 创建数据目录
mkdir -p "$DATA_DIR"

# 检查端口占用
check_port() {
    if lsof -i:$1 >/dev/null 2>&1; then
        echo -e "${RED}错误: 端口 $1 已被占用${NC}"
        return 1
    fi
    return 0
}

check_port $RPC_PORT || exit 1
check_port $P2P_PORT || exit 1

# 启动节点
echo -e "${YELLOW}启动主网节点...${NC}"

# 设置环境变量
export POLE_CHAIN_ID=$CHAIN_ID
export POLE_LOG_LEVEL=$LOG_LEVEL

# 启动
exec ./cmd/node/main.go \
    --genesis="$GENESIS_FILE" \
    --data-dir="$DATA_DIR" \
    --rpc-port="$RPC_PORT" \
    --p2p-port="$P2P_PORT" \
    --prometheus-port="$PROMETHEUS_PORT" \
    --log-level="$LOG_LEVEL"
