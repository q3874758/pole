//! Network Error Types

use thiserror::Error;

#[derive(Error, Debug)]
pub enum NetworkError {
    #[error("Not started")]
    NotStarted,
    
    #[error("Transport error: {0}")]
    TransportError(String),
    
    #[error("Behaviour error: {0}")]
    BehaviourError(String),
    
    #[error("Listen error: {0}")]
    ListenError(String),
    
    #[error("Address parse error: {0}")]
    AddrParseError(String),
    
    #[error("Publish error: {0}")]
    PublishError(String),
    
    #[error("Send error: {0}")]
    SendError(String),
    
    #[error("Peer not found")]
    PeerNotFound,
    
    #[error("Connection failed")]
    ConnectionFailed,
    
    #[error("Timeout")]
    Timeout,
    
    #[error("Message too large")]
    MessageTooLarge,
}
