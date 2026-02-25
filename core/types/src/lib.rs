//! PoLE Core Types Module
//! 
//! This module defines the core data types used throughout the PoLE blockchain.

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Block height type
pub type BlockHeight = u64;

/// Epoch number type
pub type Epoch = u64;

/// Token amount type (in smallest units, 10^-18 POLE)
pub type TokenAmount = u128;

/// Game ID type
pub type GameId = String;

/// Node ID type
pub type NodeId = [u8; 32];

/// Signature type
pub type Signature = Vec<u8>;

/// Chain ID
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ChainId {
    pub name: String,
    pub chain_id: u64,
}

impl ChainId {
    pub fn mainnet() -> Self {
        Self {
            name: "pole-mainnet".to_string(),
            chain_id: 1,
        }
    }

    pub fn testnet() -> Self {
        Self {
            name: "pole-testnet".to_string(),
            chain_id: 2,
        }
    }
}

/// Account address (32 bytes)
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Hash)]
pub struct Address(pub [u8; 32]);

impl Address {
    pub fn from_public_key(pk: &[u8]) -> Self {
        use sha2::{Sha256, Digest};
        let mut hasher = Sha256::new();
        hasher.update(pk);
        let result = hasher.finalize();
        let mut addr = [0u8; 32];
        addr.copy_from_slice(&result[..32]);
        Self(addr)
    }

    pub fn as_bytes(&self) -> &[u8] {
        &self.0
    }

    pub fn to_hex(&self) -> String {
        hex::encode(self.0)
    }
}

impl std::fmt::Display for Address {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.to_hex())
    }
}

/// Block header
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BlockHeader {
    pub height: BlockHeight,
    pub parent_hash: [u8; 32],
    pub timestamp: i64,
    pub proposer: Address,
    pub validators_hash: [u8; 32],
    pub data_hash: [u8; 32],
    pub gvs_hash: [u8; 32],
}

impl BlockHeader {
    pub fn new(height: BlockHeight, parent_hash: [u8; 32], proposer: Address) -> Self {
        Self {
            height,
            parent_hash,
            timestamp: chrono::Utc::now().timestamp(),
            proposer,
            validators_hash: [0; 32],
            data_hash: [0; 32],
            gvs_hash: [0; 32],
        }
    }
}

/// Block
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Block {
    pub header: BlockHeader,
    pub transactions: Vec<Transaction>,
    pub gvs_updates: Vec<GvsUpdate>,
}

impl Block {
    pub fn new(header: BlockHeader) -> Self {
        Self {
            header,
            transactions: Vec::new(),
            gvs_updates: Vec::new(),
        }
    }
}

/// Transaction types
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum Transaction {
    /// Token transfer
    Transfer(TransferTx),
    /// Stake tokens
    Stake(StakeTx),
    /// Unstake tokens
    Unstake(UnstakeTx),
    /// Data submission
    SubmitData(DataSubmitTx),
    /// Governance vote
    Vote(VoteTx),
    /// Contract execution
    Execute(ExecuteTx),
}

/// Transfer transaction
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TransferTx {
    pub from: Address,
    pub to: Address,
    pub amount: TokenAmount,
    pub fee: TokenAmount,
}

/// Stake transaction
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StakeTx {
    pub delegator: Address,
    pub validator: Address,
    pub amount: TokenAmount,
}

/// Unstake transaction
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UnstakeTx {
    pub delegator: Address,
    pub validator: Address,
    pub amount: TokenAmount,
}

/// Data submission transaction
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DataSubmitTx {
    pub node_id: NodeId,
    pub game_data: Vec<GameDataPoint>,
    pub signature: Signature,
}

/// Vote transaction
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VoteTx {
    pub voter: Address,
    pub proposal_id: u64,
    pub vote_option: u8, // 0: abstain, 1: yes, 2: no
    pub weight: TokenAmount,
}

/// Execute transaction
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ExecuteTx {
    pub from: Address,
    pub contract_id: u64,
    pub payload: Vec<u8>,
    pub fee: TokenAmount,
}

/// Game data point
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GameDataPoint {
    pub game_id: GameId,
    pub online_players: u64,
    pub peak_players: u64,
    pub timestamp: i64,
    pub tier: Tier,
}

/// Tier level for games
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
pub enum Tier {
    /// Tier 1: Steam API verified
    Tier1 = 1,
    /// Tier 2: Third-party data
    Tier2 = 2,
    /// Tier 3: Community verified
    Tier3 = 3,
}

impl Tier {
    pub fn weight(&self) -> f64 {
        match self {
            Tier::Tier1 => 1.0,
            Tier::Tier2 => 0.45, // Average of 0.3-0.6
            Tier::Tier3 => 0.10, // Average of 0.05-0.15
        }
    }
}

/// GVS update
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GvsUpdate {
    pub game_id: GameId,
    pub gvs: f64,
    pub tier: Tier,
    pub timestamp: i64,
}

/// Validator information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Validator {
    pub address: Address,
    pub public_key: Vec<u8>,
    pub stake: TokenAmount,
    pub delegations: TokenAmount,
    pub commission: u8,
    pub status: ValidatorStatus,
    pub moniker: String,
}

/// Validator status
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
pub enum ValidatorStatus {
    Active = 0,
    Inactive = 1,
    Jailed = 2,
    Unbonding = 3,
}

/// Node (collector) information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Node {
    pub id: NodeId,
    pub address: Address,
    pub stake: TokenAmount,
    pub reputation: f64,
    pub collected_games: Vec<GameId>,
    pub uptime: f64,
    pub last_heartbeat: i64,
}

/// Network state
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NetworkState {
    pub height: BlockHeight,
    pub epoch: Epoch,
    pub total_stake: TokenAmount,
    pub active_validators: u32,
    pub total_nodes: u32,
    pub gvs: HashMap<GameId, f64>,
}

impl Default for NetworkState {
    fn default() -> Self {
        Self {
            height: 0,
            epoch: 0,
            total_stake: 0,
            active_validators: 0,
            total_nodes: 0,
            gvs: HashMap::new(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_address_from_public_key() {
        let pk = b"test public key for address generation";
        let addr = Address::from_public_key(pk);
        assert_eq!(addr.0.len(), 32);
    }

    #[test]
    fn test_tier_weights() {
        assert_eq!(Tier::Tier1.weight(), 1.0);
        assert!(Tier::Tier2.weight() > 0.3 && Tier::Tier2.weight() < 0.6);
        assert!(Tier::Tier3.weight() > 0.05 && Tier::Tier3.weight() < 0.15);
    }
}
