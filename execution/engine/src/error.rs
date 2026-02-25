//! Engine Error Types

use thiserror::Error;

#[derive(Error, Debug)]
pub enum EngineError {
    #[error("Invalid data")]
    InvalidData,
    
    #[error("Calculation overflow")]
    Overflow,
    
    #[error("Insufficient data points")]
    InsufficientData,
    
    #[error("Configuration error")]
    ConfigError,
    
    #[error("Storage error")]
    StorageError,
}
