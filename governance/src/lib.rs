//! Governance Module
//! 
//! Implements on-chain governance for protocol upgrades and parameter changes

use crate::types::*;
use crate::error::GovernanceError;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use chrono::Utc;

pub mod error;
pub mod proposals;
pub mod voting;

pub use error::GovernanceError;

/// Governance manager
pub struct Governance {
    /// Proposals
    proposals: Arc<RwLock<HashMap<u64, Proposal>>>,
    /// Votes
    votes: Arc<RwLock<HashMap<(u64, Address), VoteRecord>>>,
    /// Configuration
    config: GovernanceConfig,
    /// Next proposal ID
    next_proposal_id: Arc<RwLock<u64>>,
    /// Governance parameters
    params: Arc<RwLock<GovernanceParams>>,
}

#[derive(Debug, Clone)]
pub struct GovernanceConfig {
    /// Minimum stake to create proposal
    pub min_proposal_stake: TokenAmount,
    /// Minimum voting period in seconds
    pub min_voting_period: i64,
    /// Quorum threshold (percentage)
    pub quorum_threshold: f64,
    /// Approval threshold (percentage)
    pub approval_threshold: f64,
    /// Execution delay in seconds
    pub execution_delay: i64,
}

impl Default for GovernanceConfig {
    fn default() -> Self {
        Self {
            min_proposal_stake: 10_000_000_000_000_000_000u128, // 10,000 POLE
            min_voting_period: 5 * 24 * 3600, // 5 days
            quorum_threshold: 0.40, // 40%
            approval_threshold: 0.50, // 50%
            execution_delay: 2 * 24 * 3600, // 2 days
        }
    }
}

/// Governance parameters (adjustable via governance)
#[derive(Debug, Clone)]
pub struct GovernanceParams {
    /// Inflation rate (0-30%)
    pub inflation_rate: f64,
    /// Decay factor
    pub decay_factor: f64,
    /// Transaction burn percentage
    pub tx_burn_percent: f64,
    /// Reward burn threshold
    pub reward_burn_threshold: TokenAmount,
    /// Reward burn percentage
    pub reward_burn_percent: f64,
    /// Minimum validator stake
    pub min_validator_stake: TokenAmount,
    /// Maximum validators
    pub max_validators: u32,
    /// Epoch length (blocks)
    pub epoch_length: u64,
    /// Unbonding period (seconds)
    pub unbonding_period: i64,
    /// Data deviation threshold
    pub data_deviation_threshold: f64,
}

impl Default for GovernanceParams {
    fn default() -> Self {
        Self {
            inflation_rate: 0.20,
            decay_factor: 0.5,
            tx_burn_percent: 0.25,
            reward_burn_threshold: 10_000_000_000_000_000_000u128,
            reward_burn_percent: 0.10,
            min_validator_stake: 10_000_000_000_000_000_000u128,
            max_validators: 21,
            epoch_length: 14400,
            unbonding_period: 21 * 24 * 3600, // 21 days
            data_deviation_threshold: 0.20,
        }
    }
}

impl Governance {
    pub fn new(config: GovernanceConfig) -> Self {
        Self {
            proposals: Arc::new(RwLock::new(HashMap::new())),
            votes: Arc::new(RwLock::new(HashMap::new())),
            config,
            next_proposal_id: Arc::new(RwLock::new(1)),
            params: Arc::new(RwLock::new(GovernanceParams::default())),
        }
    }

    /// Create a new proposal
    pub async fn create_proposal(
        &self,
        proposer: Address,
        title: String,
        description: String,
        proposal_type: ProposalType,
        data: Vec<u8>,
    ) -> Result<u64, GovernanceError> {
        // Check minimum stake (simplified)
        let stake = TokenAmount::from(0); // Would check actual stake
        
        if stake < self.config.min_proposal_stake {
            return Err(GovernanceError::InsufficientStake);
        }

        let id = {
            let mut next = self.next_proposal_id.write().await;
            let id = *next;
            *next += 1;
            id
        };

        let now = Utc::now().timestamp();
        
        let proposal = Proposal {
            id,
            proposer,
            title,
            description,
            proposal_type,
            data,
            status: ProposalStatus::Pending,
            created_at: now,
            voting_start: now,
            voting_end: now + self.config.min_voting_period,
            execute_at: 0,
            yes_votes: 0,
            no_votes: 0,
            abstain_votes: 0,
            total_voting_power: 0,
        };

        {
            let mut proposals = self.proposals.write().await;
            proposals.insert(id, proposal);
        }

        Ok(id)
    }

    /// Cast a vote
    pub async fn cast_vote(
        &self,
        proposal_id: u64,
        voter: Address,
        vote_option: VoteOption,
        weight: TokenAmount,
    ) -> Result<(), GovernanceError> {
        let mut proposals = self.proposals.write().await;
        
        let proposal = proposals
            .get_mut(&proposal_id)
            .ok_or(GovernanceError::ProposalNotFound)?;
        
        // Check voting period
        let now = Utc::now().timestamp();
        if now < proposal.voting_start || now > proposal.voting_end {
            return Err(GovernanceError::VotingClosed);
        }
        
        // Update vote counts
        match vote_option {
            VoteOption::Yes => {
                proposal.yes_votes += weight;
            }
            VoteOption::No => {
                proposal.no_votes += weight;
            }
            VoteOption::Abstain => {
                proposal.abstain_votes += weight;
            }
        }
        
        proposal.total_voting_power += weight;
        
        // Record vote
        {
            let mut votes = self.votes.write().await;
            votes.insert(
                (proposal_id, voter.clone()),
                VoteRecord {
                    proposal_id,
                    voter,
                    vote_option,
                    weight,
                    timestamp: now,
                },
            );
        }

        Ok(())
    }

    /// Tally votes and update proposal status
    pub async fn tally_votes(&self, proposal_id: u64) -> Result<ProposalStatus, GovernanceError> {
        let mut proposals = self.proposals.write().await;
        
        let proposal = proposals
            .get_mut(&proposal_id)
            .ok_or(GovernanceError::ProposalNotFound)?;
        
        let now = Utc::now().timestamp();
        
        // Check if voting period ended
        if now < proposal.voting_end {
            return Err(GovernanceError::VotingNotEnded);
        }
        
        // Calculate results
        let total = proposal.yes_votes + proposal.no_votes + proposal.abstain_votes;
        
        // Check quorum
        let quorum = if proposal.total_voting_power > 0 {
            total as f64 / proposal.total_voting_power as f64
        } else {
            0.0
        };
        
        if quorum < self.config.quorum_threshold {
            proposal.status = ProposalStatus::Rejected;
            return Ok(ProposalStatus::Rejected);
        }
        
        // Check approval
        if proposal.yes_votes as f64 > (total as f64 * self.config.approval_threshold) {
            proposal.status = ProposalStatus::Passed;
            proposal.execute_at = now + self.config.execution_delay;
        } else {
            proposal.status = ProposalStatus::Rejected;
        }
        
        Ok(proposal.status)
    }

    /// Execute a passed proposal
    pub async fn execute_proposal(&self, proposal_id: u64) -> Result<(), GovernanceError> {
        let mut proposals = self.proposals.write().await;
        
        let proposal = proposals
            .get_mut(&proposal_id)
            .ok_or(GovernanceError::ProposalNotFound)?;
        
        if proposal.status != ProposalStatus::Passed {
            return Err(GovernanceError::ProposalNotPassed);
        }
        
        let now = Utc::now().timestamp();
        
        // Check execution delay
        if now < proposal.execute_at {
            return Err(GovernanceError::ExecutionNotReady);
        }
        
        // Execute proposal (in production, would execute actual changes)
        proposal.status = ProposalStatus::Executed;
        
        Ok(())
    }

    /// Get proposal
    pub async fn get_proposal(&self, proposal_id: u64) -> Option<Proposal> {
        let proposals = self.proposals.read().await;
        proposals.get(&proposal_id).cloned()
    }

    /// Get all proposals
    pub async fn get_proposals(&self) -> Vec<Proposal> {
        let proposals = self.proposals.read().await;
        proposals.values().cloned().collect()
    }

    /// Get governance parameters
    pub async fn get_params(&self) -> GovernanceParams {
        self.params.read().await.clone()
    }

    /// Update governance parameters
    pub async fn update_params(&self, new_params: GovernanceParams) {
        let mut params = self.params.write().await;
        *params = new_params;
    }
}

/// Proposal types
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum ProposalType {
    ParameterChange,
    TextProposal,
    TreasurySpend,
    ProtocolUpgrade,
}

/// Proposal status
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum ProposalStatus {
    Pending,
    Voting,
    Passed,
    Rejected,
    Executed,
    Cancelled,
}

/// Vote options
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum VoteOption {
    Yes,
    No,
    Abstain,
}

/// Proposal
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Proposal {
    pub id: u64,
    pub proposer: Address,
    pub title: String,
    pub description: String,
    pub proposal_type: ProposalType,
    pub data: Vec<u8>,
    pub status: ProposalStatus,
    pub created_at: i64,
    pub voting_start: i64,
    pub voting_end: i64,
    pub execute_at: i64,
    pub yes_votes: TokenAmount,
    pub no_votes: TokenAmount,
    pub abstain_votes: TokenAmount,
    pub total_voting_power: TokenAmount,
}

/// Vote record
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VoteRecord {
    pub proposal_id: u64,
    pub voter: Address,
    pub vote_option: VoteOption,
    pub weight: TokenAmount,
    pub timestamp: i64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_create_proposal() {
        let governance = Governance::new(GovernanceConfig::default());
        
        let proposer = Address::from_public_key(b"proposer");
        
        let result = governance.create_proposal(
            proposer,
            "Test Proposal".to_string(),
            "Description".to_string(),
            ProposalType::TextProposal,
            vec![],
        ).await;
        
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_cast_vote() {
        let governance = Governance::new(GovernanceConfig::default());
        
        let proposer = Address::from_public_key(b"proposer");
        
        let proposal_id = governance.create_proposal(
            proposer.clone(),
            "Test".to_string(),
            "Test".to_string(),
            ProposalType::TextProposal,
            vec![],
        ).await.unwrap();
        
        let voter = Address::from_public_key(b"voter");
        
        governance.cast_vote(
            proposal_id,
            voter,
            VoteOption::Yes,
            1000,
        ).await.unwrap();
        
        let proposal = governance.get_proposal(proposal_id).await.unwrap();
        assert_eq!(proposal.yes_votes, 1000);
    }
}
