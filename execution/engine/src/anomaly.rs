//! Anomaly Detection Module
//! 
//! Detects anomalies in game data using statistical methods

use crate::types::*;
use rand::Rng;

/// Anomaly detection methods
pub struct AnomalyDetector {
    config: DetectorConfig,
}

#[derive(Debug, Clone)]
pub struct DetectorConfig {
    /// Z-score threshold for outlier detection
    pub zscore_threshold: f64,
    /// Minimum data points for analysis
    pub min_samples: usize,
    /// Window size for moving average
    pub window_size: usize,
}

impl Default for DetectorConfig {
    fn default() -> Self {
        Self {
            zscore_threshold: 3.0,
            min_samples: 10,
            window_size: 5,
        }
    }
}

impl AnomalyDetector {
    pub fn new(config: DetectorConfig) -> Self {
        Self { config }
    }

    /// Detect anomalies using z-score method
    pub fn detect_zscore(&self, data: &[u64]) -> Vec<usize> {
        if data.len() < self.config.min_samples {
            return Vec::new();
        }

        let mean = data.iter().sum::<u64>() as f64 / data.len() as f64;
        
        let variance = data.iter()
            .map(|x| {
                let diff = *x as f64 - mean;
                diff * diff
            })
            .sum::<f64>() / data.len() as f64;
        
        let std_dev = variance.sqrt();
        
        if std_dev == 0.0 {
            return Vec::new();
        }

        data.iter()
            .enumerate()
            .filter(|(_, &x)| {
                let zscore = (x as f64 - mean).abs() / std_dev;
                zscore > self.config.zscore_threshold
            })
            .map(|(i, _)| i)
            .collect()
    }

    /// Detect sudden changes in player count
    pub fn detect_sudden_changes(&self, data: &[u64], threshold: f64) -> Vec<(usize, u64, u64)> {
        if data.len() < 2 {
            return Vec::new();
        }

        let mut anomalies = Vec::new();
        
        for i in 1..data.len() {
            let prev = data[i - 1] as f64;
            let curr = data[i] as f64;
            
            if prev == 0.0 {
                continue;
            }
            
            let change = ((curr - prev) / prev).abs();
            
            if change > threshold {
                anomalies.push((i, data[i - 1], data[i]));
            }
        }
        
        anomalies
    }

    /// Detect unusual patterns using moving average deviation
    pub fn detect_ma_deviation(&self, data: &[u64]) -> Vec<usize> {
        if data.len() < self.config.window_size {
            return Vec::new();
        }

        let mut anomalies = Vec::new();
        
        for i in self.config.window_size..data.len() {
            let window = &data[i - self.config.window_size..i];
            let ma = window.iter().sum::<u64>() as f64 / window.len() as f64;
            
            let deviation = (data[i] as f64 - ma).abs() / ma;
            
            if deviation > 0.5 { // 50% deviation
                anomalies.push(i);
            }
        }
        
        anomalies
    }

    /// Validate node data submission
    pub fn validate_node_data(&self, node_data: &[GameDataPoint], network_data: &[GameDataPoint]) -> ValidationResult {
        if node_data.is_empty() {
            return ValidationResult {
                is_valid: false,
                anomalies: vec!["No data submitted".to_string()],
                deviation: 1.0,
            };
        }

        let mut anomalies = Vec::new();
        
        // Calculate average deviation
        let node_avg: f64 = node_data.iter().map(|d| d.online_players as f64).sum::<f64>() 
            / node_data.len() as f64;
        
        let network_avg: f64 = network_data.iter().map(|d| d.online_players as f64).sum::<f64>() 
            / network_data.len() as f64;
        
        let deviation = if network_avg > 0.0 {
            (node_avg - network_avg).abs() / network_avg
        } else {
            0.0
        };

        // Check for outliers
        let player_counts: Vec<u64> = node_data.iter().map(|d| d.online_players).collect();
        let outliers = self.detect_zscore(&player_counts);
        
        if !outliers.is_empty() {
            anomalies.push(format!("{} outliers detected", outliers.len()));
        }

        // Check for sudden changes
        let sudden_changes = self.detect_sudden_changes(&player_counts, 0.5);
        if !sudden_changes.is_empty() {
            anomalies.push(format!("{} sudden changes detected", sudden_changes.len()));
        }

        // Check deviation threshold
        if deviation > 0.5 {
            anomalies.push(format!("High deviation from network: {:.1}%", deviation * 100.0));
        }

        ValidationResult {
            is_valid: anomalies.is_empty(),
            anomalies,
            deviation,
        }
    }
}

/// Validation result
#[derive(Debug, Clone)]
pub struct ValidationResult {
    pub is_valid: bool,
    pub anomalies: Vec<String>,
    pub deviation: f64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_zscore_detection() {
        let detector = AnomalyDetector::new(DetectorConfig::default());
        
        let data = vec![
            100, 102, 101, 99, 103, 500, 101, 100, 102, 99
        ];
        
        let anomalies = detector.detect_zscore(&data);
        assert!(!anomalies.is_empty());
    }

    #[test]
    fn test_sudden_changes() {
        let detector = AnomalyDetector::new(DetectorConfig::default());
        
        let data = vec![100, 100, 100, 500, 100, 100];
        
        let changes = detector.detect_sudden_changes(&data, 0.5);
        assert!(!changes.is_empty());
    }
}
