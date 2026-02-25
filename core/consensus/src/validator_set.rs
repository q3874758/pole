//! Validator Set Module

use crate::types::*;
use std::collections::HashMap;

/// Validator set management
pub struct ValidatorSet {
    validators: HashMap<Address, Validator>,
    active_validators: Vec<Address>,
}

impl ValidatorSet {
    pub fn new() -> Self {
        Self {
            validators: HashMap::new(),
            active_validators: Vec::new(),
        }
    }

    /// Add or update a validator
    pub fn add_validator(&mut self, validator: Validator) {
        if validator.status == ValidatorStatus::Active {
            if !self.active_validators.contains(&validator.address) {
                self.active_validators.push(validator.address.clone());
            }
        }
        self.validators.insert(validator.address.clone(), validator);
    }

    /// Remove a validator
    pub fn remove_validator(&mut self, address: &Address) -> Option<Validator> {
        self.active_validators.retain(|a| a != address);
        self.validators.remove(address)
    }

    /// Get validator by address
    pub fn get_validator(&self, address: &Address) -> Option<&Validator> {
        self.validators.get(address)
    }

    /// Check if address is an active validator
    pub fn is_active_validator(&self, address: &Address) -> bool {
        self.validators
            .get(address)
            .map(|v| v.status == ValidatorStatus::Active)
            .unwrap_or(false)
    }

    /// Get all validators
    pub fn get_all_validators(&self) -> Vec<Validator> {
        self.validators.values().cloned().collect()
    }

    /// Get active validators
    pub fn get_active_validators(&self) -> Vec<Validator> {
        self.validators
            .values()
            .filter(|v| v.status == ValidatorStatus::Active)
            .cloned()
            .collect()
    }

    /// Get validator count
    pub fn validator_count(&self) -> usize {
        self.validators.len()
    }

    /// Get active validator count
    pub fn active_count(&self) -> usize {
        self.active_validators.len()
    }

    /// Get total staked amount
    pub fn total_stake(&self) -> TokenAmount {
        self.validators
            .values()
            .map(|v| v.stake + v.delegations)
            .sum()
    }

    /// Update validator status
    pub fn update_status(&mut self, address: &Address, status: ValidatorStatus) -> bool {
        if let Some(validator) = self.validators.get_mut(address) {
            let was_active = validator.status == ValidatorStatus::Active;
            validator.status = status;
            
            let is_active = status == ValidatorStatus::Active;
            
            if was_active && !is_active {
                self.active_validators.retain(|a| a != address);
            } else if !was_active && is_active {
                if !self.active_validators.contains(address) {
                    self.active_validators.push(address.clone());
                }
            }
            return true;
        }
        false
    }

    /// Slash validator
    pub fn slash_validator(&mut self, address: &Address, slash_amount: TokenAmount) -> bool {
        if let Some(validator) = self.validators.get_mut(address) {
            if validator.stake >= slash_amount {
                validator.stake -= slash_amount;
                return true;
            }
        }
        false
    }
}

impl Default for ValidatorSet {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_validator_set() {
        let mut set = ValidatorSet::new();
        
        let validator = Validator {
            address: Address::from_public_key(b"test validator"),
            public_key: b"test pubkey".to_vec(),
            stake: 1000,
            delegations: 500,
            commission: 10,
            status: ValidatorStatus::Active,
            moniker: "test".to_string(),
        };
        
        set.add_validator(validator.clone());
        
        assert!(set.is_active_validator(&validator.address));
        assert_eq!(set.validator_count(), 1);
        assert_eq!(set.active_count(), 1);
        assert_eq!(set.total_stake(), 1500);
    }
}
