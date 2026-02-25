//! PoS (Proof of Stake) Consensus Module

use crate::types::*;
use crate::error::ConsensusError;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;

/// PoS consensus state
pub struct PosConsensus {
    /// Last committed block height
    last_height: BlockHeight,
    /// Validator voting history
    votes: HashMap<Address, Vec<Vote>>,
    /// Slashing records
    slashes: HashMap<Address, Vec<SlashRecord>>,
}

#[derive(Debug, Clone)]
pub struct Vote {
    pub height: BlockHeight,
    pub round: u64,
    pub block_hash: [u8; 32],
    pub vote_type: VoteType,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum VoteType {
    Prevote,
    Precommit,
}

#[derive(Debug, Clone)]
pub struct SlashRecord {
    pub height: BlockHeight,
    pub reason: SlashReason,
    pub slash_amount: TokenAmount,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SlashReason {
    DoubleSign,
    Unavailable,
    MaliciousBehavior,
}

impl PosConsensus {
    pub fn new() -> Self {
        Self {
            last_height: 0,
            votes: HashMap::new(),
            slashes: HashMap::new(),
        }
    }

    /// Record a new block
    pub fn record_block(&mut self, height: BlockHeight) -> Result<(), ConsensusError> {
        if height <= self.last_height {
            return Err(ConsensusError::InvalidBlock);
        }
        self.last_height = height;
        Ok(())
    }

    /// Record a vote
    pub fn record_vote(&mut self, validator: Address, vote: Vote) -> Result<(), ConsensusError> {
        self.votes
            .entry(validator)
            .or_insert_with(Vec::new)
            .push(vote);
        Ok(())
    }

    /// Slash a validator
    pub fn slash(&mut self, validator: Address, record: SlashRecord) {
        self.slashes
            .entry(validator)
            .or_insert_with(Vec::new)
            .push(record);
    }

    /// Get slash count for validator
    pub fn get_slash_count(&self, validator: &Address) -> usize {
        self.slashes.get(validator).map(|v| v.len()).unwrap_or(0)
    }

    /// Get last height
    pub fn get_last_height(&self) -> BlockHeight {
        self.last_height
    }
}

impl Default for PosConsensus {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_pos_consensus() {
        let mut pos = PosConsensus::new();
        assert_eq!(pos.get_last_height(), 0);
        
        pos.record_block(1).unwrap();
        assert_eq!(pos.get_last_height(), 1);
    }
}
