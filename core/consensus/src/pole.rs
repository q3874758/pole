//! PoLE (Proof of Live Engagement) Consensus Module
//! 
//! Handles data verification consensus for game data.

use crate::types::*;
use crate::error::ConsensusError;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;

/// Data verification vote
#[derive(Debug, Clone)]
pub struct DataVote {
    pub voter: NodeId,
    pub data_hash: [u8; 32],
    pub vote: bool,
    pub timestamp: i64,
    pub stake: TokenAmount,
}

/// Verification result
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum VerificationResult {
    Approved,
    Rejected,
    Pending,
    Invalid,
}

/// PoLE consensus for data verification
pub struct PoleConsensus {
    /// Pending votes for each data hash
    votes: HashMap<[u8; 32], Vec<DataVote>>,
    /// Verification results
    results: HashMap<[u8; 32], VerificationResult>,
    /// Vote counts
    vote_counts: HashMap<[u8; 32], (u64, u64)>, // (yes, no)
    /// Quorum threshold (percentage)
    quorum: f64,
    /// Approval threshold (percentage)
    approval_threshold: f64,
}

impl PoleConsensus {
    pub fn new() -> Self {
        Self {
            votes: HashMap::new(),
            results: HashMap::new(),
            vote_counts: HashMap::new(),
            quorum: 0.67, // 2/3 majority
            approval_threshold: 0.67,
        }
    }

    /// Submit a vote for data verification
    pub async fn submit_vote(
        &mut self,
        voter: Address,
        data_hash: [u8; 32],
        vote: bool,
    ) -> Result<(), ConsensusError> {
        let node_id = Self::address_to_node_id(&voter);
        
        let data_vote = DataVote {
            voter: node_id,
            data_hash,
            vote,
            timestamp: chrono::Utc::now().timestamp(),
            stake: 0, // Would be fetched from stake state
        };

        self.votes
            .entry(data_hash)
            .or_insert_with(Vec::new)
            .push(data_vote);

        // Update vote counts
        let counts = self.vote_counts.entry(data_hash).or_insert((0, 0));
        if vote {
            counts.0 += 1;
        } else {
            counts.1 += 1;
        }

        // Check if we can determine result
        self.check_and_set_result(data_hash).await;

        Ok(())
    }

    /// Check and set verification result
    async fn check_and_set_result(&mut self, data_hash: [u8; 32]) {
        let votes = match self.votes.get(&data_hash) {
            Some(v) => v,
            None => return,
        };

        let total_votes = votes.len() as f64;
        let yes_votes = votes.iter().filter(|v| v.vote).count() as f64;
        
        // Check quorum
        if total_votes < 3 {
            // Need at least 3 votes
            return;
        }

        let yes_ratio = yes_votes / total_votes;
        
        let result = if yes_ratio >= self.approval_threshold {
            VerificationResult::Approved
        } else if (1.0 - yes_ratio) >= self.approval_threshold {
            VerificationResult::Rejected
        } else {
            VerificationResult::Pending
        };

        self.results.insert(data_hash, result);
    }

    /// Get verification result
    pub fn get_result(&self, data_hash: [u8; 32]) -> Option<bool> {
        match self.results.get(&data_hash) {
            Some(VerificationResult::Approved) => Some(true),
            Some(VerificationResult::Rejected) => Some(false),
            Some(VerificationResult::Pending) | Some(VerificationResult::Invalid) | None => None,
        }
    }

    /// Get vote status
    pub fn get_vote_status(&self, data_hash: [u8; 32]) -> Option<VerificationResult> {
        self.results.get(&data_hash).copied()
    }

    /// Get vote counts
    pub fn get_vote_counts(&self, data_hash: [u8; 32]) -> Option<(u64, u64)> {
        self.vote_counts.get(&data_hash).copied()
    }

    /// Get total votes for data
    pub fn get_total_votes(&self, data_hash: [u8; 32]) -> usize {
        self.votes.get(&data_hash).map(|v| v.len()).unwrap_or(0)
    }

    /// Convert address to node ID
    fn address_to_node_id(address: &Address) -> NodeId {
        let mut node_id = [0u8; 32];
        let bytes = address.as_bytes();
        node_id.copy_from_slice(bytes);
        node_id
    }

    /// Set quorum threshold
    pub fn set_quorum(&mut self, quorum: f64) {
        self.quorum = quorum.clamp(0.5, 1.0);
    }

    /// Set approval threshold
    pub fn set_approval_threshold(&mut self, threshold: f64) {
        self.approval_threshold = threshold.clamp(0.5, 1.0);
    }
}

impl Default for PoleConsensus {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_pole_consensus() {
        let mut pole = PoleConsensus::new();
        
        let data_hash = [0u8; 32];
        let voter1 = Address::from_public_key(b"voter1");
        
        // Submit votes
        pole.submit_vote(voter1.clone(), data_hash, true).await.unwrap();
        
        assert_eq!(pole.get_total_votes(data_hash), 1);
        // Result pending - need more votes
        assert!(pole.get_result(data_hash).is_none());
    }
}
