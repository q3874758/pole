//! P2P Network Module
//! 
//! Implements peer-to-peer communication using libp2p

use crate::types::*;
use crate::error::NetworkError;
use async_trait::async_trait;
use libp2p::{
    core::upgrade,
    futures::StreamExt,
    mplex,
    noise,
    tcp, quic,
    identity,
    PeerId,
    Multiaddr,
    Swarm,
    gossipsub,
    mdns,
    swarm::SwarmEvent,
};
use std::sync::Arc;
use tokio::sync::RwLock;
use std::collections::HashMap;

pub mod error;
pub mod behaviours;
pub mod protocols;

pub use error::NetworkError;

/// P2P Network manager
pub struct P2pNetwork {
    /// Local peer ID
    local_peer_id: PeerId,
    /// Swarm
    swarm: Arc<RwLock<Option<Swarm<P2pBehaviour>>>>,
    /// Connected peers
    connected_peers: Arc<RwLock<HashMap<PeerId, PeerInfo>>>,
    /// Configuration
    config: NetworkConfig,
    /// Message handlers
    message_handlers: Arc<RwLock<HashMap<String, Box<dyn MessageHandler>>>>,
}

#[derive(Debug, Clone)]
pub struct NetworkConfig {
    /// Listen addresses
    pub listen_addresses: Vec<String>,
    /// Boot nodes
    pub boot_nodes: Vec<(PeerId, Multiaddr)>,
    /// Enable mDNS discovery
    pub enable_mdns: bool,
    /// Enable gossipsub
    pub enable_gossipsub: bool,
    /// Gossipsub heartbeat
    pub gossipsub_heartbeat: u64,
    /// Maximum connections
    pub max_connections: u32,
}

impl Default for NetworkConfig {
    fn default() -> Self {
        Self {
            listen_addresses: vec!["/ip4/0.0.0.0/tcp/0".to_string()],
            boot_nodes: Vec::new(),
            enable_mdns: true,
            enable_gossipsub: true,
            gossipsub_heartbeat: 5,
            max_connections: 100,
        }
    }
}

/// Peer information
#[derive(Debug, Clone)]
pub struct PeerInfo {
    pub peer_id: PeerId,
    pub addresses: Vec<Multiaddr>,
    pub connected_at: i64,
    pub last_message: i64,
}

/// Message handler trait
#[async_trait]
pub trait MessageHandler: Send + Sync {
    /// Handle incoming message
    async fn handle(&self, from: PeerId, data: &[u8]) -> Result<Vec<u8>, NetworkError>;
    
    /// Get topic name
    fn topic(&self) -> &str;
}

impl P2pNetwork {
    pub fn new(config: NetworkConfig) -> Self {
        // Generate random keypair for local node
        let keypair = identity::Keypair::generate_ed25519();
        let local_peer_id = PeerId::from(keypair.public());
        
        Self {
            local_peer_id,
            swarm: Arc::new(RwLock::new(None)),
            connected_peers: Arc::new(RwLock::new(HashMap::new())),
            config,
            message_handlers: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Get local peer ID
    pub fn local_peer_id(&self) -> PeerId {
        self.local_peer_id
    }

    /// Register message handler
    pub fn register_handler(&self, handler: Box<dyn MessageHandler>) {
        let topic = handler.topic().to_string();
        let mut handlers = self.message_handlers.blocking_write();
        handlers.insert(topic, handler);
    }

    /// Start the network
    pub async fn start(&self) -> Result<(), NetworkError> {
        // Create transport
        let transport = tcp::Config::new()
            .upgrade(upgrade::Version::V1)
            .authenticate(noise::Config::new(&identity::Keypair::generate_ed25519()).map_err(|e| NetworkError::TransportError(e.to_string()))?)
            .multiplex(mplex::Config::new())
            .boxed();

        // Create behaviour
        let behaviour = P2pBehaviour::new(self.config.clone()).map_err(|e| NetworkError::BehaviourError(e.to_string()))?;

        // Create swarm
        let mut swarm = Swarm::new(transport, behaviour, self.local_peer_id);

        // Listen on addresses
        for addr in &self.config.listen_addresses {
            let addr: Multiaddr = addr.parse().map_err(|e| NetworkError::AddrParseError(e.to_string()))?;
            Swarm::listen_on(&mut swarm, addr).map_err(|e| NetworkError::ListenError(e.to_string()))?;
        }

        // Store swarm
        let mut swarm_guard = self.swarm.write().await;
        *swarm_guard = Some(swarm);

        Ok(())
    }

    /// Run network event loop
    pub async fn run(&self) {
        let mut swarm_guard = self.swarm.write().await;
        
        if let Some(ref mut swarm) = *swarm_guard {
            loop {
                tokio::select! {
                    event = swarm.next() => {
                        if let Some(event) = event {
                            self.handle_swarm_event(event).await;
                        }
                    }
                    _ = tokio::time::sleep(std::time::Duration::from_secs(1)) => {
                        // Keep alive
                    }
                }
            }
        }
    }

    /// Handle swarm event
    async fn handle_swarm_event(&self, event: SwarmEvent<P2pEvent>) {
        match event {
            SwarmEvent::NewListenAddr { address, .. } => {
                tracing::info!("Listening on {}", address);
            }
            SwarmEvent::ConnectionEstablished { peer_id, .. } => {
                tracing::info!("Connected to {}", peer_id);
                
                let mut peers = self.connected_peers.write().await;
                peers.insert(peer_id, PeerInfo {
                    peer_id,
                    addresses: Vec::new(),
                    connected_at: chrono::Utc::now().timestamp(),
                    last_message: chrono::Utc::now().timestamp(),
                });
            }
            SwarmEvent::ConnectionClosed { peer_id, .. } => {
                tracing::info!("Disconnected from {}", peer_id);
                
                let mut peers = self.connected_peers.write().await;
                peers.remove(&peer_id);
            }
            SwarmEvent::Behaviour(P2pEvent::Gossipsub(gossipsub::Event::Message { message, .. })) => {
                self.handle_gossipsub_message(message).await;
            }
            _ => {}
        }
    }

    /// Handle gossipsub message
    async fn handle_gossipsub_message(&self, message: gossipsub::Message) {
        let topic = message.topic.to_string();
        let handlers = self.message_handlers.read().await;
        
        if let Some(handler) = handlers.get(&topic) {
            // Handle message
            let _ = handler.handle(message.source, &message.data).await;
        }
    }

    /// Publish message to topic
    pub async fn publish(&self, topic: &str, data: Vec<u8>) -> Result<(), NetworkError> {
        let swarm = self.swarm.read().await;
        
        if let Some(ref swarm) = *swarm {
            let topic = gossipsub::IdentTopic::new(topic);
            
            swarm.behaviour_mut().gossipsub.publish(topic, data)
                .map_err(|e| NetworkError::PublishError(e.to_string()))?;
            
            Ok(())
        } else {
            Err(NetworkError::NotStarted)
        }
    }

    /// Send direct message to peer
    pub async fn send_to(&self, peer: &PeerId, data: Vec<u8>) -> Result<(), NetworkError> {
        let swarm = self.swarm.read().await;
        
        if let Some(ref swarm) = *swarm {
            swarm.behaviour_mut().floodsub.publish(gossipsub::IdentTopic::new("pole-direct"), data)
                .map_err(|e| NetworkError::SendError(e.to_string()))?;
            
            Ok(())
        } else {
            Err(NetworkError::NotStarted)
        }
    }

    /// Get connected peers
    pub async fn get_connected_peers(&self) -> Vec<PeerId> {
        let peers = self.connected_peers.read().await;
        peers.keys().cloned().collect()
    }

    /// Get peer count
    pub async fn peer_count(&self) -> usize {
        let peers = self.connected_peers.read().await;
        peers.len()
    }
}

/// P2P behaviour (combines all network behaviours)
pub struct P2pBehaviour {
    pub gossipsub: gossipsub::Behaviour,
    pub mdns: Option<mdns::Behaviour>,
}

impl P2pBehaviour {
    pub fn new(config: NetworkConfig) -> Result<Self, Box<dyn std::error::Error>> {
        // Create gossipsub
        let gossipsub_config = gossipsub::Config::default();
        let gossipsub = gossipsub::Behaviour::new(
            gossipsub::MessageAuthenticity::Anonymous,
            gossipsub_config,
        )?;

        // Create mdns
        let mdns = if config.enable_mdns {
            Some(mdns::Behaviour::new(
                mdns::Config::default(),
                config.max_connections as usize,
            )?)
        } else {
            None
        };

        Ok(Self { gossipsub, mdns })
    }
}

/// P2P events
#[derive(Debug)]
pub enum P2pEvent {
    Gossipsub(gossipsub::Event),
    Mdns(mdns::Event),
}

impl From<gossipsub::Event> for P2pEvent {
    fn from(event: gossipsub::Event) -> Self {
        P2pEvent::Gossipsub(event)
    }
}

impl From<mdns::Event> for P2pEvent {
    fn from(event: mdns::Event) -> Self {
        P2pEvent::Mdns(event)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_network_creation() {
        let network = P2pNetwork::new(NetworkConfig::default());
        
        assert!(!network.local_peer_id().to_bytes().is_empty());
    }
}
