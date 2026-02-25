//! Token Burn Module

use crate::types::*;
use std::sync::Arc;
use tokio::sync::RwLock;

/// Token burn mechanism
pub struct TokenBurn {
    /// Total burned tokens
    total_burned: Arc<RwLock<TokenAmount>>,
    /// Burn configuration
    config: BurnConfig,
}

#[derive(Debug, Clone)]
pub struct BurnConfig {
    /// Transaction fee burn percentage
    pub fee_burn_percent: f64,
    /// Reward burn threshold
    pub reward_burn_threshold: TokenAmount,
    /// Reward burn percentage
    pub reward_burn_percent: f64,
    /// Governance burn percentage
    pub governance_burn_percent: f64,
}

impl Default for BurnConfig {
    fn default() -> Self {
        Self {
            fee_burn_percent: 0.25,        // 25%
            reward_burn_threshold: 10_000_000_000_000_000_000u128, // 10,000 POLE
            reward_burn_percent: 0.10,     // 10%
            governance_burn_percent: 0.01,  // 1%
        }
    }
}

impl TokenBurn {
    pub fn new(config: BurnConfig) -> Self {
        Self {
            total_burned: Arc::new(RwLock::new(0)),
            config,
        }
    }

    /// Calculate fee burn amount
    pub fn calculate_fee_burn(&self, fee: TokenAmount) -> TokenAmount {
        (fee as f64 * self.config.fee_burn_percent) as TokenAmount
    }

    /// Calculate reward burn amount
    pub fn calculate_reward_burn(&self, reward: TokenAmount) -> TokenAmount {
        if reward > self.config.reward_burn_threshold {
            let excess = reward - self.config.reward_burn_threshold;
            (excess as f64 * self.config.reward_burn_percent) as TokenAmount
        } else {
            0
        }
    }

    /// Calculate governance burn amount
    pub fn calculate_governance_burn(&self, locked: TokenAmount) -> TokenAmount {
        (locked as f64 * self.config.governance_burn_percent) as TokenAmount
    }

    /// Burn tokens
    pub async fn burn(&self, amount: TokenAmount) {
        let mut total = self.total_burned.write().await;
        *total += amount;
    }

    /// Get total burned
    pub async fn get_total_burned(&self) -> TokenAmount {
        *self.total_burned.read().await
    }

    /// Process transaction fee
    pub async fn process_fee(&self, fee: TokenAmount) -> (TokenAmount, TokenAmount) {
        let burn_amount = self.calculate_fee_burn(fee);
        let remaining = fee - burn_amount;
        
        self.burn(burn_amount).await;
        
        (burn_amount, remaining)
    }

    /// Process reward
    pub async fn process_reward(&self, reward: TokenAmount) -> (TokenAmount, TokenAmount) {
        let burn_amount = self.calculate_reward_burn(reward);
        let remaining = reward - burn_amount;
        
        self.burn(burn_amount).await;
        
        (burn_amount, remaining)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_fee_burn() {
        let burn = TokenBurn::new(BurnConfig::default());
        
        let fee = 1000u128;
        let (burn_amount, remaining) = (
            burn.calculate_fee_burn(fee),
            fee - burn.calculate_fee_burn(fee)
        );
        
        assert_eq!(burn_amount, 250);
        assert_eq!(remaining, 750);
    }

    #[test]
    fn test_reward_burn_below_threshold() {
        let burn = TokenBurn::new(BurnConfig::default());
        
        let reward = 5_000_000_000_000_000_000u128; // 5000 POLE < threshold
        let burn_amount = burn.calculate_reward_burn(reward);
        
        assert_eq!(burn_amount, 0);
    }

    #[test]
    fn test_reward_burn_above_threshold() {
        let burn = TokenBurn::new(BurnConfig::default());
        
        let reward = 15_000_000_000_000_000_000u128; // 15000 POLE > threshold
        let burn_amount = burn.calculate_reward_burn(reward);
        
        // (15000 - 10000) * 10% = 500
        assert_eq!(burn_amount, 500_000_000_000_000_000u128);
    }
}
