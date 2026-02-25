//! Governance Error Types

use thiserror::Error;

#[derive(Error, Debug)]
pub enum GovernanceError {
    #[error("Proposal not found")]
    ProposalNotFound,
    
    #[error("Insufficient stake")]
    InsufficientStake,
    
    #[error("Voting closed")]
    VotingClosed,
    
    #[error("Voting not ended")]
    VotingNotEnded,
    
    #[error("Proposal not passed")]
    ProposalNotPassed,
    
    #[error("Execution not ready")]
    ExecutionNotReady,
    
    #[error("Duplicate vote")]
    DuplicateVote,
    
    #[error("Invalid proposal")]
    InvalidProposal,
}
