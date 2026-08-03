//! OS credential-store boundary. M3 supplies platform keyring adapters.

pub trait SecretStore: Send + Sync {
    fn get(&self, account: &str) -> Result<Option<String>, SecretStoreError>;
    fn set(&self, account: &str, secret: &str) -> Result<(), SecretStoreError>;
    fn delete(&self, account: &str) -> Result<(), SecretStoreError>;
}

#[derive(Debug)]
pub enum SecretStoreError {
    Unavailable,
    Backend(String),
}

pub struct UnavailableSecretStore;

impl SecretStore for UnavailableSecretStore {
    fn get(&self, _account: &str) -> Result<Option<String>, SecretStoreError> {
        Err(SecretStoreError::Unavailable)
    }

    fn set(&self, _account: &str, _secret: &str) -> Result<(), SecretStoreError> {
        Err(SecretStoreError::Unavailable)
    }

    fn delete(&self, _account: &str) -> Result<(), SecretStoreError> {
        Err(SecretStoreError::Unavailable)
    }
}
