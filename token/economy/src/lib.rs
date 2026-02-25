//! Token Economy Module
//! 
//! Implements token minting, burning, and economic model

use crate::types::*;
use crate::error::EconomyError;
use std::sync::Arc;
use tokio::sync::RwLock;
use std::collections::HashMap;

pub mod error;
pub mod mint;
pub mod burn;

pub use error::EconomyError;

/// Token economy manager
pub struct TokenEconomy {
    /// Total token supply
    total_supply: TokenAmount,
    /// Circulating supply
    circulating_supply: TokenAmount,
    /// Current inflation rate (annual)
    inflation_rate: f64,
    /// Current year
    current_year: u32,
    /// Token balances
    balances: Arc<RwLock<HashMap<Address, TokenAmount>>>,
    /// Delegations
    delegations: Arc<RwLock<HashMap<(Address, Address), TokenAmount>>>, // (delegator, validator) -> amount
    /// Configuration
    config: EconomyConfig,
    /// Historical supply data
    supply_history: Arc<RwLock<Vec<SupplyRecord>>>,
}

#[derive(Debug, Clone)]
pub struct EconomyConfig {
    /// Total supply (1 billion POLE)
    pub total_supply: TokenAmount,
    /// Initial inflation rate (20%)
    pub initial_inflation: f64,
    /// Decay factor (0.5 = halve every 2 years)
    pub decay_factor: f64,
    /// Block time in seconds
    pub block_time: u64,
    /// Blocks per year
    pub blocks_per_year: u64,
    /// Validator reward share
    pub validator_reward_share: f64,
    /// Delegator reward share
    pub delegator_reward_share: f64,
}

impl Default for EconomyConfig {
    fn default() -> Self {
        Self {
            total_supply: 1_000_000_000_000_000_000_000u128, // 1 billion * 10^18
            initial_inflation: 0.20,
            decay_factor: 0.5,
            block_time: 3,
            blocks_per_year: 31_536_000 / 3, // ~10.5 million
            validator_reward_share: 0.70,
            delegator_reward_share: 0.30,
        }
    }
}

/// Supply record for history
#[derive(Debug, Clone)]
pub struct SupplyRecord {
    pub height: BlockHeight,
    pub total_supply: TokenAmount,
    pub circulating_supply: TokenAmount,
    pub inflation_rate: f64,
    pub timestamp: i64,
}

impl TokenEconomy {
    pub fn new(config: EconomyConfig) -> Self {
        Self {
            total_supply: config.total_supply,
            circulating_supply: 0,
            inflation_rate: config.initial_inflation,
            current_year: 1,
            balances: Arc::new(RwLock::new(HashMap::new())),
            delegations: Arc::new(RwLock::new(HashMap::new())),
            config,
            supply_history: Arc::new(RwLock::new(Vec::new())),
        }
    }

    /// Get current inflation rate
    pub fn get_inflation_rate(&self) -> f64 {
        self.inflation_rate
    }

    /// Calculate inflation rate for current year
    pub fn calculate_inflation(&self) -> f64 {
        // Annual_Inflation_Rate = Initial_Inflation × (1/2)^(Year / 2)
        self.config.initial_inflation * self.config.decay_factor.powf(self.current_year as f64 / 2.0)
    }

    /// Calculate block reward
    pub fn calculate_block_reward(&self) -> TokenAmount {
        // Annual reward = Total Supply * Inflation Rate
        let annual_reward = (self.total_supply as f64 * self.inflation_rate) as TokenAmount;
        
        // Block reward = Annual Reward / Blocks per year
        annual_reward / self.config.blocks_per_year
    }

    /// Mint new tokens for block reward
    pub async fn mint_block_reward(&mut self, validator: &Address) -> Result<(TokenAmount, TokenAmount), EconomyError> {
        let reward = self.calculate_block_reward();
        
        // Split between validator and delegators
        let validator_reward = (reward as f64 * self.config.validator_reward_share) as TokenAmount;
        let delegator_reward = (reward as f64 * self.config.delegator_reward_share) as TokenAmount;
        
        // Add to validator balance
        {
            let mut balances = self.balances.write().await;
            *balances.entry(validator.clone()).or_insert(0) += validator_reward;
        }
        
        // Update totals
        self.total_supply += reward;
        self.circulating_supply += reward;
        
        // Record history
        self.record_supply().await;
        
        Ok((validator_reward, delegator_reward))
    }

    /// Transfer tokens
    pub async fn transfer(&self, from: &Address, to: &Address, amount: TokenAmount) -> Result<(), EconomyError> {
        let mut balances = self.balances.write().await;
        
        let from_balance = balances.get(from).copied().unwrap_or(0);
        
        if from_balance < amount {
            return Err(EconomyError::InsufficientBalance);
        }
        
        *balances.get_mut(from).unwrap() -= amount;
        *balances.entry(to.clone()).or_insert(0) += amount;
        
        Ok(())
    }

    /// Stake tokens
    pub async fn stake(&self, delegator: &Address, validator: &Address, amount: TokenAmount) -> Result<(), EconomyError> {
        // Check balance
        {
            let balances = self.balances.read().await;
            let balance = balances.get(delegator).copied().unwrap_or(0);
            
            if balance < amount {
                return Err(EconomyError::InsufficientBalance);
            }
        }
        
        // Deduct from balance
        {
            let mut balances = self.balances.write().await;
            *balances.get_mut(delegator).unwrap() -= amount;
        }
        
        // Add to delegation
        {
            let mut delegations = self.delegations.write().await;
            *delegations.entry((delegator.clone(), validator.clone())).or_insert(0) += amount;
        }
        
        Ok(())
    }

    /// Unstake tokens
    pub async fn unstake(&self, delegator: &Address, validator: &Address, amount: TokenAmount) -> Result<(), EconomyError> {
        // Check delegation
        {
            let delegations = self.delegations.read().await;
            let current = delegations.get(&(delegator.clone(), validator.clone())).copied().unwrap_or(0);
            
            if current < amount {
                return Err(EconomyError::InsufficientBalance);
            }
        }
        
        // Remove from delegation
        {
            let mut delegations = self.delegations.write().await;
            *delegations.get_mut(&(delegator.clone(), validator.clone())).unwrap() -= amount;
        }
        
        // Return to balance (with unlock period in production)
        {
            let mut balances = self.balances.write().await;
            *balances.entry(delegator.clone()).or_insert(0) += amount;
        }
        
        Ok(())
    }

    /// Get balance
    pub async fn get_balance(&self, address: &Address) -> TokenAmount {
        let balances = self.balances.read().await;
        balances.get(address).copied().unwrap_or(0)
    }

    /// Get total staked
    pub async fn get_total_staked(&self) -> TokenAmount {
        let delegations = self.delegations.read().await;
        delegations.values().sum()
    }

    /// Record supply change
    async fn record_supply(&self) {
        let record = SupplyRecord {
            height: 0, // Would be passed in
            total_supply: self.total_supply,
            circulating_supply: self.circulating_supply,
            inflation_rate: self.inflation_rate,
            timestamp: chrono::Utc::now().timestamp(),
        };
        
        let mut history = self.supply_history.write().await;
        history.push(record);
    }

    /// Get supply history
    pub async fn get_supply_history(&self) -> Vec<SupplyRecord> {
        let history = self.supply_history.read().await;
        history.clone()
    }

    /// Advance year
    pub fn advance_year(&mut self) {
        self.current_year += 1;
        self.inflation_rate = self.calculate_inflation();
    }
}

/// Token supply info
#[derive(Debug, Clone)]
pub struct SupplyInfo {
    pub total_supply: TokenAmount,
    pub circulating_supply: TokenAmount,
    pub staked_supply: TokenAmount,
    pub inflation_rate: f64,
}

impl TokenEconomy {
    pub async fn get_supply_info(&self) -> SupplyInfo {
        let staked = self.get_total_staked().await;
        let circulating = self.circulating_supply;
        
        SupplyInfo {
            total_supply: self.total_supply,
            circulating_supply: circulating,
            staked_supply: staked,
            inflation_rate: self.inflation_rate,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_inflation_calculation() {
        let economy = TokenEconomy::new(EconomyConfig::default());
        
        assert_eq!(economy.get_inflation_rate(), 0.20);
    }

    #[tokio::test]
    async fn test_transfer() {
        let economy = TokenEconomy::new(EconomyConfig::default());
        
        let addr1 = Address::from_public_key(b"address1");
        let addr2 = Address::from_public_key(b"address2");
        
        // Initialize balance
        {
            let mut balances = economy.balances.write().await;
            balances.insert(addr1.clone(), 1000);
        }
        
        // Transfer
        economy.transfer(&addr1, &addr2, 500).await.unwrap();
        
        let balance1 = economy.get_balance(&addr1).await;
        let balance2 = economy.get_balance(&addr2).await;
        
        assert_eq!(balance1, 500);
        assert_eq!(balance2, 500);
    }
}
