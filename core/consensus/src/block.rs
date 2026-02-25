//! Block Production Module

use crate::types::*;
use crate::error::ConsensusError;
use std::sync::Arc;
use tokio::sync::RwLock;

/// Block producer
pub struct BlockProducer {
    /// Current chain height
    height: BlockHeight,
    /// Block hash of last block
    last_block_hash: [u8; 32],
    /// Proposer address
    proposer: Address,
    /// Configuration
    config: BlockProducerConfig,
}

#[derive(Debug, Clone)]
pub struct BlockProducerConfig {
    /// Maximum transactions per block
    pub max_txs: usize,
    /// Maximum data points per block
    pub max_data_points: usize,
    /// Block gas limit
    pub gas_limit: u64,
}

impl Default for BlockProducerConfig {
    fn default() -> Self {
        Self {
            max_txs: 1000,
            max_data_points: 5000,
            gas_limit: 50_000_000,
        }
    }
}

impl BlockProducer {
    pub fn new(proposer: Address) -> Self {
        Self {
            height: 0,
            last_block_hash: [0u8; 32],
            proposer,
            config: BlockProducerConfig::default(),
        }
    }

    /// Create a new block
    pub fn create_block(
        &mut self,
        transactions: Vec<Transaction>,
        gvs_updates: Vec<GvsUpdate>,
    ) -> Result<Block, ConsensusError> {
        // Validate transactions count
        if transactions.len() > self.config.max_txs {
            return Err(ConsensusError::TooManyTransactions);
        }

        // Validate data points count
        if gvs_updates.len() > self.config.max_data_points {
            return Err(ConsensusError::TooManyDataPoints);
        }

        // Increment height
        self.height += 1;

        // Create header
        let mut header = BlockHeader::new(
            self.height,
            self.last_block_hash,
            self.proposer.clone(),
        );

        // Calculate hashes
        header.data_hash = Self::calculate_data_hash(&transactions);
        header.gvs_hash = Self::calculate_gvs_hash(&gvs_updates);
        header.validators_hash = self.calculate_validators_hash();

        // Update last block hash
        self.last_block_hash = Self::calculate_block_hash(&header);

        Ok(Block {
            header,
            transactions,
            gvs_updates,
        })
    }

    /// Calculate block hash
    fn calculate_block_hash(header: &BlockHeader) -> [u8; 32] {
        use sha2::{Sha256, Digest};
        let mut hasher = Sha256::new();
        
        // Include all header fields in hash
        hasher.update(header.height.to_le_bytes());
        hasher.update(&header.parent_hash);
        hasher.update(header.timestamp.to_le_bytes());
        hasher.update(header.proposer.as_bytes());
        hasher.update(&header.validators_hash);
        hasher.update(&header.data_hash);
        hasher.update(&header.gvs_hash);
        
        let result = hasher.finalize();
        let mut hash = [0u8; 32];
        hash.copy_from_slice(&result[..32]);
        hash
    }

    /// Calculate data hash
    fn calculate_data_hash(txs: &[Transaction]) -> [u8; 32] {
        use sha2::{Sha256, Digest};
        let mut hasher = Sha256::new();
        
        for tx in txs {
            let tx_data = serde_json::to_vec(tx).unwrap_or_default();
            hasher.update(&tx_data);
        }
        
        let result = hasher.finalize();
        let mut hash = [0u8; 32];
        hash.copy_from_slice(&result[..32]);
        hash
    }

    /// Calculate GVS hash
    fn calculate_gvs_hash(updates: &[GvsUpdate]) -> [u8; 32] {
        use sha2::{Sha256, Digest};
        let mut hasher = Sha256::new();
        
        for update in updates {
            let data = format!(
                "{}:{}:{}:{}",
                update.game_id, update.gvs, update.tier as u8, update.timestamp
            );
            hasher.update(data.as_bytes());
        }
        
        let result = hasher.finalize();
        let mut hash = [0u8; 32];
        hash.copy_from_slice(&result[..32]);
        hash
    }

    /// Calculate validators hash (placeholder)
    fn calculate_validators_hash(&self) -> [u8; 32] {
        // In production, this would hash the current validator set
        [0u8; 32]
    }

    /// Get current height
    pub fn height(&self) -> BlockHeight {
        self.height
    }

    /// Get last block hash
    pub fn last_block_hash(&self) -> [u8; 32] {
        self.last_block_hash
    }

    /// Set proposer
    pub fn set_proposer(&mut self, proposer: Address) {
        self.proposer = proposer;
    }

    /// Update height from state
    pub fn sync_height(&mut self, height: BlockHeight, block_hash: [u8; 32]) {
        if height > self.height {
            self.height = height;
            self.last_block_hash = block_hash;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_block_production() {
        let proposer = Address::from_public_key(b"proposer");
        let mut producer = BlockProducer::new(proposer.clone());
        
        let block = producer.create_block(
            vec![Transaction::Transfer(TransferTx {
                from: Address::from_public_key(b"from"),
                to: Address::from_public_key(b"to"),
                amount: 1000,
                fee: 1,
            })],
            vec![],
        ).unwrap();
        
        assert_eq!(block.header.height, 1);
    }
}
