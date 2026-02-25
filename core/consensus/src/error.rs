//! Consensus Error Types

use thiserror::Error;

#[derive(Error, Debug)]
pub enum ConsensusError {
    #[error("No proposer available")]
    NoProposer,
    
    #[error("Invalid proposer")]
    InvalidProposer,
    
    #[error("Invalid block signature")]
    InvalidSignature,
    
    #[error("Duplicate vote")]
    DuplicateVote,
    
    #[error("Insufficient stake")]
    InsufficientStake,
    
    #[error("Validator not found")]
    ValidatorNotFound,
    
    #[error("Validator is jailed")]
    ValidatorJailed,
    
    #[error("Consensus timeout")]
    Timeout,
    
    #[error("Not enough votes")]
    InsufficientVotes,
    
    #[error("Invalid vote")]
    InvalidVote,
    
    #[error("Already committed")]
    AlreadyCommitted,
    
    #[error("IO error: {0}")]
    IoError(#[from] std::io::Error),
}
