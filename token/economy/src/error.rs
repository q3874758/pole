//! Economy Error Types

use thiserror::Error;

#[derive(Error, Debug)]
pub enum EconomyError {
    #[error("Insufficient balance")]
    InsufficientBalance,
    
    #[error("Invalid amount")]
    InvalidAmount,
    
    #[error("Overflow")]
    Overflow,
    
    #[error("Underflow")]
    Underflow,
    
    #[error("Stake not found")]
    StakeNotFound,
    
    #[error("Validator not found")]
    ValidatorNotFound,
    
    #[error("Token transfer failed")]
    TransferFailed,
}
