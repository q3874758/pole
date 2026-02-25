//! Rewards Distribution Module

use crate::types::*;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;

/// Reward distribution manager
pub struct RewardDistributor {
    /// Configuration
    config: RewardConfig,
    /// Node scores (node_id -> score)
    node_scores: Arc<RwLock<HashMap<NodeId, NodeScore>>>,
    /// Pending rewards
    pending_rewards: Arc<RwLock<HashMap<Address, TokenAmount>>>,
}

#[derive(Debug, Clone)]
pub struct RewardConfig {
    /// Base reward per data point
    pub base_reward: TokenAmount,
    /// Tier 1 weight
    pub tier1_weight: f64,
    /// Tier 2 weight
    pub tier2_weight: f64,
    /// Tier 3 weight
    pub tier3_weight: f64,
    /// Quality factor multiplier
    pub quality_multiplier: f64,
    /// Exploration bonus multiplier
    pub exploration_bonus: f64,
}

impl Default for RewardConfig {
    fn default() -> Self {
        Self {
            base_reward: 1_000_000_000_000_000u128, // 0.001 POLE per data point
            tier1_weight: 1.0,
            tier2_weight: 0.45,
            tier3_weight: 0.10,
            quality_multiplier: 1.5,
            exploration_bonus: 2.0,
        }
    }
}

/// Node score tracking
#[derive(Debug, Clone)]
pub struct NodeScore {
    pub node_id: NodeId,
    /// Total data points submitted
    pub data_points: u64,
    /// Quality score (0-1)
    pub quality: f64,
    /// Uptime percentage
    pub uptime: f64,
    /// Games covered
    pub games_covered: u32,
    /// Exploration bonus earned
    pub exploration_bonus: TokenAmount,
}

impl RewardDistributor {
    pub fn new(config: RewardConfig) -> Self {
        Self {
            config,
            node_scores: Arc::new(RwLock::new(HashMap::new())),
            pending_rewards: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Calculate reward for data contribution
    pub fn calculate_data_reward(
        &self,
        tier: Tier,
        quality_factor: f64,
        is_exploration: bool,
    ) -> TokenAmount {
        // Get tier weight
        let tier_weight = match tier {
            Tier::Tier1 => self.config.tier1_weight,
            Tier::Tier2 => self.config.tier2_weight,
            Tier::Tier3 => self.config.tier3_weight,
        };

        // Base calculation
        let mut reward = self.config.base_reward as f64;

        // Apply tier weight
        reward *= tier_weight;

        // Apply quality multiplier
        reward *= 1.0 + (quality_factor * (self.config.quality_multiplier - 1.0));

        // Apply exploration bonus
        if is_exploration {
            reward *= self.config.exploration_bonus;
        }

        reward as TokenAmount
    }

    /// Calculate node score
    pub async fn update_node_score(
        &self,
        node_id: NodeId,
        data_points_delta: u64,
        quality: f64,
    ) {
        let mut scores = self.node_scores.write().await;
        
        let score = scores.entry(node_id).or_insert(NodeScore {
            node_id,
            data_points: 0,
            quality: 0.5,
            uptime: 1.0,
            games_covered: 0,
            exploration_bonus: 0,
        });
        
        score.data_points += data_points_delta;
        score.quality = (score.quality + quality) / 2.0;
    }

    /// Calculate reward for epoch
    pub async fn calculate_epoch_reward(
        &self,
        node_id: &NodeId,
        epoch_reward: TokenAmount,
    ) -> TokenAmount {
        let scores = self.node_scores.read().await;
        
        let node_score = match scores.get(node_id) {
            Some(s) => s,
            None => return 0,
        };
        
        // Calculate total network score
        let total_score: f64 = scores.values()
            .map(|s| s.data_points as f64 * s.quality)
            .sum();
        
        if total_score == 0.0 {
            return 0;
        }
        
        // Node's share
        let node_contribution = node_score.data_points as f64 * node_score.quality;
        let share = node_contribution / total_score;
        
        (epoch_reward as f64 * share) as TokenAmount
    }

    /// Distribute rewards
    pub async fn distribute_rewards(
        &self,
        rewards: HashMap<Address, TokenAmount>,
    ) -> Result<(), RewardError> {
        let mut pending = self.pending_rewards.write().await;
        
        for (address, amount) in rewards {
            *pending.entry(address).or_insert(0) += amount;
        }
        
        Ok(())
    }

    /// Claim pending rewards
    pub async fn claim_rewards(&self, address: &Address) -> TokenAmount {
        let mut pending = self.pending_rewards.write().await;
        pending.remove(address).unwrap_or(0)
    }

    /// Get pending rewards
    pub async fn get_pending_rewards(&self, address: &Address) -> TokenAmount {
        let pending = self.pending_rewards.read().await;
        pending.get(address).copied().unwrap_or(0)
    }

    /// Get node score
    pub async fn get_node_score(&self, node_id: &NodeId) -> Option<NodeScore> {
        let scores = self.node_scores.read().await;
        scores.get(node_id).cloned()
    }
}

/// Calculate validator rewards
pub fn calculate_validator_reward(
    total_stake: TokenAmount,
    validator_stake: TokenAmount,
    block_reward: TokenAmount,
) -> TokenAmount {
    if total_stake == 0 || validator_stake == 0 {
        return 0;
    }
    
    let share = validator_stake as f64 / total_stake as f64;
    (block_reward as f64 * share) as TokenAmount
}

/// Calculate delegator rewards
pub fn calculate_delegator_reward(
    delegator_stake: TokenAmount,
    validator_total_stake: TokenAmount,
    validator_reward: TokenAmount,
    commission: u8,
) -> TokenAmount {
    if validator_total_stake == 0 || delegator_stake == 0 {
        return 0;
    }
    
    // Validator's share of rewards
    let validator_share = validator_reward as f64 
        * (validator_total_stake as f64 / validator_total_stake as f64);
    
    // Delegator's share (after commission)
    let commission_factor = 1.0 - (commission as f64 / 100.0);
    let delegator_share = validator_share * commission_factor 
        * (delegator_stake as f64 / validator_total_stake as f64);
    
    delegator_share as TokenAmount
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tier_weights() {
        let distributor = RewardDistributor::new(RewardConfig::default());
        
        let tier1_reward = distributor.calculate_data_reward(Tier::Tier1, 1.0, false);
        let tier2_reward = distributor.calculate_data_reward(Tier::Tier2, 1.0, false);
        let tier3_reward = distributor.calculate_data_reward(Tier::Tier3, 1.0, false);
        
        assert!(tier1_reward > tier2_reward);
        assert!(tier2_reward > tier3_reward);
    }

    #[test]
    fn test_exploration_bonus() {
        let distributor = RewardDistributor::new(RewardConfig::default());
        
        let normal_reward = distributor.calculate_data_reward(Tier::Tier1, 1.0, false);
        let exploration_reward = distributor.calculate_data_reward(Tier::Tier1, 1.0, true);
        
        assert!(exploration_reward > normal_reward);
    }

    #[test]
    fn test_validator_reward() {
        let total = 1_000_000u128;
        let validator = 100_000u128;
        let block_reward = 1000u128;
        
        let reward = calculate_validator_reward(total, validator, block_reward);
        
        assert_eq!(reward, 100);
    }
}
