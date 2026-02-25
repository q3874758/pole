//! PoLE Consensus Module
//! 
//! Implements PoS + PoLE hybrid consensus mechanism.

use crate::types::*;
use crate::error::ConsensusError;
use async_trait::async_trait;
use std::sync::Arc;
use tokio::sync::RwLock;
use std::collections::HashMap;

pub mod error;
pub mod pos;
pub mod pole;
pub mod block;
pub mod validator_set;

pub use error::ConsensusError;
pub use pos::PosConsensus;
pub use pole::PoleConsensus;
pub use block::{BlockProducer, Block};
pub use validator_set::ValidatorSet;

/// Consensus trait for different consensus mechanisms
#[async_trait]
pub trait Consensus: Send + Sync {
    /// Get current consensus state
    async fn get_state(&self) -> ConsensusState;
    
    /// Process a new block
    async fn process_block(&self, block: &Block) -> Result<(), ConsensusError>;
    
    /// Check if we're in a valid state to produce blocks
    async fn can_produce_block(&self) -> bool;
    
    /// Get the proposer for current height
    async fn get_proposer(&self, height: BlockHeight) -> Option<Address>;
}

/// Consensus state
#[derive(Debug, Clone)]
pub struct ConsensusState {
    pub height: BlockHeight,
    pub round: u64,
    pub step: ConsensusStep,
    pub proposer: Option<Address>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ConsensusStep {
    Propose,
    Vote,
    Commit,
    NewRound,
}

/// Hybrid consensus combining PoS and PoLE
pub struct HybridConsensus {
    pos: Arc<RwLock<PosConsensus>>,
    pole: Arc<RwLock<PoleConsensus>>,
    validator_set: Arc<RwLock<ValidatorSet>>,
    state: Arc<RwLock<ConsensusState>>,
    config: ConsensusConfig,
}

#[derive(Debug, Clone)]
pub struct ConsensusConfig {
    /// Block time in seconds
    pub block_time: u64,
    /// Timeout for propose phase (in seconds)
    pub propose_timeout: u64,
    /// Timeout for vote phase (in seconds)
    pub vote_timeout: u64,
    /// Minimum stake to become validator
    pub min_stake: TokenAmount,
    /// Maximum number of validators
    pub max_validators: u32,
    /// Number of blocks per epoch
    pub blocks_per_epoch: u64,
}

impl Default for ConsensusConfig {
    fn default() -> Self {
        Self {
            block_time: 3,
            propose_timeout: 2,
            vote_timeout: 2,
            min_stake: 10_000_000_000_000_000_000u128, // 10,000 POLE
            max_validators: 21,
            blocks_per_epoch: 14400, // ~12 hours at 3s block time
        }
    }
}

impl HybridConsensus {
    pub fn new(config: ConsensusConfig) -> Self {
        Self {
            pos: Arc::new(RwLock::new(PosConsensus::new())),
            pole: Arc::new(RwLock::new(PoleConsensus::new())),
            validator_set: Arc::new(RwLock::new(ValidatorSet::new())),
            state: Arc::new(RwLock::new(ConsensusState {
                height: 0,
                round: 0,
                step: ConsensusStep::NewRound,
                proposer: None,
            })),
            config,
        }
    }

    /// Update validator set
    pub async fn update_validators(&self, validators: Vec<Validator>) {
        let mut set = self.validator_set.write().await;
        for v in validators {
            set.add_validator(v);
        }
    }

    /// Get validator set
    pub async fn get_validators(&self) -> Vec<Validator> {
        let set = self.validator_set.read().await;
        set.get_all_validators()
    }

    /// Check if validator is active
    pub async fn is_active_validator(&self, address: &Address) -> bool {
        let set = self.validator_set.read().await;
        set.is_active_validator(address)
    }

    /// Submit vote for data verification
    pub async fn submit_data_vote(
        &self,
        voter: Address,
        data_hash: [u8; 32],
        vote: bool,
    ) -> Result<(), ConsensusError> {
        let pole = self.pole.write().await;
        pole.submit_vote(voter, data_hash, vote).await
    }

    /// Get data verification result
    pub async fn get_data_verification_result(&self, data_hash: [u8; 32]) -> Option<bool> {
        let pole = self.pole.read().await;
        pole.get_result(data_hash)
    }

    /// Calculate proposer based on PoS
    pub async fn calculate_proposer(&self, height: BlockHeight) -> Option<Address> {
        let pos = self.pos.read().await;
        let validators = self.validator_set.read().await;
        
        let active: Vec<&Validator> = validators
            .get_all_validators()
            .iter()
            .filter(|v| v.status == ValidatorStatus::Active)
            .collect();
        
        if active.is_empty() {
            return None;
        }
        
        // Deterministic proposer selection based on height and stake
        let total_stake: TokenAmount = active.iter().map(|v| v.stake + v.delegations).sum();
        if total_stake == 0 {
            return None;
        }
        
        let mut seed = height as u128;
        for v in &active {
            seed = seed.wrapping_add(v.stake as u128);
        }
        
        let proposer_index = (seed % active.len() as u128) as usize;
        Some(active[proposer_index].address.clone())
    }

    /// Advance consensus to next step
    pub async fn advance_step(&self) {
        let mut state = self.state.write().await;
        state.step = match state.step {
            ConsensusStep::NewRound => ConsensusStep::Propose,
            ConsensusStep::Propose => ConsensusStep::Vote,
            ConsensusStep::Vote => ConsensusStep::Commit,
            ConsensusStep::Commit => {
                state.height += 1;
                state.round = 0;
                ConsensusStep::NewRound
            }
        };
    }

    /// Get current consensus state
    pub async fn get_state(&self) -> ConsensusState {
        let state = self.state.read().await;
        state.clone()
    }
}

#[async_trait]
impl Consensus for HybridConsensus {
    async fn get_state(&self) -> ConsensusState {
        self.get_state().await
    }

    async fn process_block(&self, block: &Block) -> Result<(), ConsensusError> {
        // Verify block proposer
        let proposer = self.calculate_proposer(block.header.height).await
            .ok_or(ConsensusError::NoProposer)?;
        
        if proposer != block.header.proposer {
            return Err(ConsensusError::InvalidProposer);
        }

        // Verify signatures (simplified - in production would verify actual signatures)
        // Update PoS state
        {
            let mut pos = self.pos.write().await;
            pos.record_block(block.header.height)?;
        }

        // Update state
        {
            let mut state = self.state.write().await;
            state.height = block.header.height;
            state.step = ConsensusStep::NewRound;
            state.round += 1;
            state.proposer = Some(proposer);
        }

        Ok(())
    }

    async fn can_produce_block(&self) -> bool {
        let state = self.state.read().await;
        state.step == ConsensusStep::Propose
    }

    async fn get_proposer(&self, height: BlockHeight) -> Option<Address> {
        self.calculate_proposer(height).await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_consensus_creation() {
        let config = ConsensusConfig::default();
        let consensus = HybridConsensus::new(config);
        let state = consensus.get_state().await;
        
        assert_eq!(state.height, 0);
        assert_eq!(state.step, ConsensusStep::NewRound);
    }

    #[tokio::test]
    async fn test_proposer_calculation() {
        let config = ConsensusConfig::default();
        let consensus = HybridConsensus::new(config);
        
        let proposer = consensus.calculate_proposer(1).await;
        // No validators yet, should return None
        assert!(proposer.is_none());
    }
}
