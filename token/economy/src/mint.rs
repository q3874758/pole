//! Token Mint Module

use crate::types::*;
use std::sync::Arc;
use tokio::sync::RwLock;

/// Inflation minting module
pub struct TokenMinter {
    /// Configuration
    config: MintConfig,
    /// Current year
    current_year: u32,
    /// Epoch reward pool
    epoch_reward: Arc<RwLock<TokenAmount>>,
}

#[derive(Debug, Clone)]
pub struct MintConfig {
    /// Initial inflation rate (annual)
    pub initial_inflation: f64,
    /// Decay factor (halving period in years)
    pub decay_factor: f64,
    /// Target TNGV (Total Network Game Value)
    pub target_tngv: f64,
    /// Block time in seconds
    pub block_time: u64,
}

impl Default for MintConfig {
    fn default() -> Self {
        Self {
            initial_inflation: 0.20,
            decay_factor: 0.5,
            target_tngv: 1_000_000.0,
            block_time: 3,
        }
    }
}

impl TokenMinter {
    pub fn new(config: MintConfig) -> Self {
        Self {
            config,
            current_year: 1,
            epoch_reward: Arc::new(RwLock::new(0)),
        }
    }

    /// Calculate inflation rate for year
    pub fn get_inflation_rate(&self) -> f64 {
        // Annual_Inflation_Rate = Initial_Inflation × (1/2)^(Year / 2)
        self.config.initial_inflation 
            * self.config.decay_factor.powf(self.current_year as f64 / 2.0)
    }

    /// Calculate annual reward
    pub fn calculate_annual_reward(&self, total_supply: TokenAmount) -> TokenAmount {
        let rate = self.get_inflation_rate();
        (total_supply as f64 * rate) as TokenAmount
    }

    /// Calculate block reward
    pub fn calculate_block_reward(&self, total_supply: TokenAmount) -> TokenAmount {
        let annual = self.calculate_annual_reward(total_supply);
        let blocks_per_year = 31_536_000 / self.config.block_time;
        annual / blocks_per_year
    }

    /// Calculate epoch reward
    pub fn calculate_epoch_reward(&self, total_supply: TokenAmount, blocks_per_epoch: u64) -> TokenAmount {
        let block_reward = self.calculate_block_reward(total_supply);
        block_reward * blocks_per_epoch
    }

    /// Adjust reward based on TNGV (difficulty adjustment)
    pub fn adjust_for_tngv(&self, base_reward: TokenAmount, current_tngv: f64) -> TokenAmount {
        // Current_Reward = Base_Reward × (Target_TNGV / Current_TNGV)^0.5
        let ratio = self.config.target_tngv / current_tngv.max(1.0);
        let adjustment = ratio.sqrt().clamp(0.5, 2.0); // Clamp between 0.5x and 2x
        
        (base_reward as f64 * adjustment) as TokenAmount
    }

    /// Advance to next year
    pub fn advance_year(&mut self) {
        self.current_year += 1;
    }

    /// Get current year
    pub fn get_current_year(&self) -> u32 {
        self.current_year
    }

    /// Set epoch reward
    pub async fn set_epoch_reward(&self, amount: TokenAmount) {
        let mut reward = self.epoch_reward.write().await;
        *reward = amount;
    }

    /// Get epoch reward
    pub async fn get_epoch_reward(&self) -> TokenAmount {
        *self.epoch_reward.read().await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_inflation_decay() {
        let minter = TokenMinter::new(MintConfig::default());
        
        assert_eq!(minter.get_inflation_rate(), 0.20);
    }

    #[test]
    fn test_block_reward() {
        let minter = TokenMinter::new(MintConfig::default());
        
        let total_supply = 1_000_000_000_000_000_000_000u128; // 1 billion
        let block_reward = minter.calculate_block_reward(total_supply);
        
        assert!(block_reward > 0);
    }

    #[test]
    fn test_tngv_adjustment() {
        let minter = TokenMinter::new(MintConfig::default());
        
        let base = 1000u128;
        
        // Higher TNGV = lower reward
        let adjusted_high = minter.adjust_for_tngv(base, 2_000_000.0);
        assert!(adjusted_high < base);
        
        // Lower TNGV = higher reward
        let adjusted_low = minter.adjust_for_tngv(base, 500_000.0);
        assert!(adjusted_low > base);
    }
}
