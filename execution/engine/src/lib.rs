//! Execution Engine - GVS Calculator
//! 
//! Calculates Game Value Score based on game data

use crate::types::*;
use crate::error::EngineError;
use std::collections::HashMap;

pub mod error;
pub mod anomaly;

pub use error::EngineError;

/// GVS Calculator
/// 
/// Implements the Game Value Score calculation:
/// GVS = Base_GLV × Tier_Coefficient × Time_Decay × Coverage_Bonus
pub struct GvsCalculator {
    config: GvsConfig,
    /// Historical GVS for time decay calculation
    history: HashMap<GameId, Vec<GvsRecord>>,
    /// Node coverage tracking
    coverage: HashMap<GameId, u32>,
}

#[derive(Debug, Clone)]
pub struct GvsConfig {
    /// Base GLV (Game List Value) multiplier
    pub base_multiplier: f64,
    /// Time decay half-life in hours
    pub decay_half_life: f64,
    /// Coverage bonus threshold (number of nodes)
    pub coverage_threshold: u32,
    /// Maximum coverage bonus
    pub max_coverage_bonus: f64,
    /// Rolling window for GVS calculation (hours)
    pub window_hours: u32,
}

impl Default for GvsConfig {
    fn default() -> Self {
        Self {
            base_multiplier: 1.0,
            decay_half_life: 24.0, // 24 hours
            coverage_threshold: 5,
            max_coverage_bonus: 1.5,
            window_hours: 24,
        }
    }
}

/// GVS calculation record
#[derive(Debug, Clone)]
pub struct GvsRecord {
    pub game_id: GameId,
    pub gvs: f64,
    pub timestamp: i64,
}

impl GvsCalculator {
    pub fn new(config: GvsConfig) -> Self {
        Self {
            config,
            history: HashMap::new(),
            coverage: HashMap::new(),
        }
    }

    /// Calculate GVS for a single game
    pub fn calculate(&self, data_points: &[GameDataPoint]) -> f64 {
        if data_points.is_empty() {
            return 0.0;
        }

        // Base GLV calculation
        let base_glv = self.calculate_base_glv(data_points);
        
        // Get tier coefficient
        let tier_coefficient = self.get_tier_coefficient(data_points);
        
        // Time decay factor
        let time_decay = self.calculate_time_decay(data_points);
        
        // Coverage bonus
        let coverage_bonus = self.calculate_coverage_bonus(data_points.first().map(|d| &d.game_id));

        // Final GVS
        let gvs = base_glv * tier_coefficient * time_decay * coverage_bonus;

        gvs.max(0.0)
    }

    /// Calculate Base GLV (Game List Value)
    fn calculate_base_glv(&self, data_points: &[GameDataPoint]) -> f64 {
        if data_points.is_empty() {
            return 0.0;
        }

        let n = data_points.len() as f64;
        
        // Average online players
        let avg_online: f64 = data_points.iter().map(|d| d.online_players as f64).sum::<f64>() / n;
        
        // Peak players
        let peak = data_points.iter().map(|d| d.peak_players as f64).max_by(|a, b| a.partial_cmp(b).unwrap()).unwrap_or(0.0);
        
        // Calculate GLV using logarithmic scale
        let glv = (avg_online + 1.0).ln() * (peak + 1.0).sqrt() * self.config.base_multiplier;
        
        glv
    }

    /// Get tier coefficient
    fn get_tier_coefficient(&self, data_points: &[GameDataPoint]) -> f64 {
        if let Some(first) = data_points.first() {
            first.tier.weight()
        } else {
            0.1
        }
    }

    /// Calculate time decay factor
    fn calculate_time_decay(&self, data_points: &[GameDataPoint]) -> f64 {
        use std::time::{SystemTime, UNIX_EPOCH};
        
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        
        // Use the most recent data point's age
        let latest = data_points.iter().max_by_key(|d| d.timestamp).unwrap();
        let age_hours = (now - latest.timestamp) as f64 / 3600.0;
        
        // Exponential decay: 2^(-age/half_life)
        let decay = 2.0_f64.powf(-age_hours / self.config.decay_half_life);
        
        decay.max(0.1) // Minimum 10% weight
    }

    /// Calculate coverage bonus
    fn calculate_coverage_bonus(&self, game_id: Option<&GameId>) -> f64 {
        let game_id = match game_id {
            Some(id) => id,
            None => return 1.0,
        };

        let node_count = self.coverage.get(game_id).copied().unwrap_or(0);

        if node_count >= self.config.coverage_threshold {
            let ratio = node_count as f64 / self.config.coverage_threshold as f64;
            let bonus = 1.0 + (ratio * 0.5).min(self.config.max_coverage_bonus - 1.0);
            bonus
        } else {
            1.0
        }
    }

    /// Update coverage count for a game
    pub fn update_coverage(&mut self, game_id: GameId) {
        *self.coverage.entry(game_id).or_insert(0) += 1;
    }

    /// Record GVS calculation in history
    pub fn record_gvs(&mut self, game_id: GameId, gvs: f64) {
        use std::time::{SystemTime, UNIX_EPOCH};
        
        let timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        
        let record = GvsRecord {
            game_id: game_id.clone(),
            gvs,
            timestamp,
        };
        
        self.history
            .entry(game_id)
            .or_insert_with(Vec::new)
            .push(record);
    }

    /// Get historical GVS for a game
    pub fn get_history(&self, game_id: &GameId) -> Vec<GvsRecord> {
        self.history.get(game_id).cloned().unwrap_or_default()
    }

    /// Calculate average GVS over time window
    pub fn get_average_gvs(&self, game_id: &GameId, hours: u32) -> Option<f64> {
        use std::time::{SystemTime, UNIX_EPOCH};
        
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        
        let cutoff = now - (hours * 3600) as i64;
        
        let history = self.history.get(game_id)?;
        
        let relevant: Vec<_> = history
            .iter()
            .filter(|r| r.timestamp > cutoff)
            .collect();
        
        if relevant.is_empty() {
            return None;
        }
        
        let sum: f64 = relevant.iter().map(|r| r.gvs).sum();
        Some(sum / relevant.len() as f64)
    }

    /// Calculate trend (positive = growing, negative = shrinking)
    pub fn calculate_trend(&self, game_id: &GameId) -> Option<f64> {
        let history = self.history.get(game_id)?;
        
        if history.len() < 2 {
            return None;
        }
        
        // Compare recent average to older average
        let mid = history.len() / 2;
        let recent: f64 = history[mid..].iter().map(|r| r.gvs).sum::<f64>() / (history.len() - mid) as f64;
        let older: f64 = history[..mid].iter().map(|r| r.gvs).sum::<f64>() / mid as f64;
        
        if older == 0.0 {
            return None;
        }
        
        Some((recent - older) / older)
    }
}

/// Batch GVS calculator for multiple games
pub struct BatchGvsCalculator {
    calculator: GvsCalculator,
}

impl BatchGvsCalculator {
    pub fn new(config: GvsConfig) -> Self {
        Self {
            calculator: GvsCalculator::new(config),
        }
    }

    /// Calculate GVS for multiple games
    pub fn calculate_batch(&mut self, game_data: HashMap<GameId, Vec<GameDataPoint>>) -> HashMap<GameId, f64> {
        let mut results = HashMap::new();
        
        for (game_id, data) in game_data {
            let gvs = self.calculator.calculate(&data);
            self.calculator.record_gvs(game_id.clone(), gvs);
            results.insert(game_id, gvs);
        }
        
        results
    }

    /// Get all GVS sorted by value
    pub fn get_top_games(&self, limit: usize) -> Vec<(GameId, f64)> {
        // This would need to access stored results
        Vec::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_gvs_calculation() {
        let calculator = GvsCalculator::new(GvsConfig::default());
        
        let data_points = vec![
            GameDataPoint {
                game_id: "test_game".to_string(),
                online_players: 1000,
                peak_players: 1500,
                timestamp: 1000,
                tier: Tier::Tier1,
            },
            GameDataPoint {
                game_id: "test_game".to_string(),
                online_players: 1200,
                peak_players: 1500,
                timestamp: 1100,
                tier: Tier::Tier1,
            },
        ];
        
        let gvs = calculator.calculate(&data_points);
        assert!(gvs > 0.0);
    }

    #[test]
    fn test_tier_weights() {
        let calculator = GvsCalculator::new(GvsConfig::default());
        
        let tier1_data = vec![GameDataPoint {
            game_id: "game1".to_string(),
            online_players: 100,
            peak_players: 150,
            timestamp: 1000,
            tier: Tier::Tier1,
        }];
        
        let tier2_data = vec![GameDataPoint {
            game_id: "game2".to_string(),
            online_players: 100,
            peak_players: 150,
            timestamp: 1000,
            tier: Tier::Tier2,
        }];
        
        let gvs1 = calculator.calculate(&tier1_data);
        let gvs2 = calculator.calculate(&tier2_data);
        
        assert!(gvs1 > gvs2);
    }
}
