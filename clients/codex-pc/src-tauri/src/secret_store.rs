//! OS credential-store boundary.

const SERVICE_NAME: &str = "cn.ldxp.sub2api.codexpc";

pub trait SecretStore: Send + Sync {
    fn is_supported(&self) -> bool;
    fn get(&self, account: &str) -> Result<Option<String>, SecretStoreError>;
    fn set(&self, account: &str, secret: &str) -> Result<(), SecretStoreError>;
    fn delete(&self, account: &str) -> Result<(), SecretStoreError>;
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum SecretStoreError {
    Unavailable,
    InvalidInput,
    Backend,
}

impl SecretStoreError {
    pub fn public_code(&self) -> &'static str {
        match self {
            Self::Unavailable => "SECURE_STORE_UNAVAILABLE",
            Self::InvalidInput => "SECURE_STORE_INVALID_INPUT",
            Self::Backend => "SECURE_STORE_ERROR",
        }
    }
}

pub struct UnavailableSecretStore;

impl SecretStore for UnavailableSecretStore {
    fn is_supported(&self) -> bool {
        false
    }

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

/// Uses the operating system's native credential store. The account label is
/// metadata; callers must never place a password or token in it.
pub struct NativeSecretStore {
    service: String,
}

impl Default for NativeSecretStore {
    fn default() -> Self {
        Self {
            service: SERVICE_NAME.to_owned(),
        }
    }
}

impl NativeSecretStore {
    fn entry(&self, account: &str) -> Result<keyring::Entry, SecretStoreError> {
        validate_account(account)?;
        keyring::Entry::new(&self.service, account).map_err(map_keyring_error)
    }
}

impl SecretStore for NativeSecretStore {
    fn is_supported(&self) -> bool {
        true
    }

    fn get(&self, account: &str) -> Result<Option<String>, SecretStoreError> {
        match self.entry(account)?.get_password() {
            Ok(secret) => Ok(Some(secret)),
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(error) => Err(map_keyring_error(error)),
        }
    }

    fn set(&self, account: &str, secret: &str) -> Result<(), SecretStoreError> {
        if secret.is_empty() {
            return Err(SecretStoreError::InvalidInput);
        }
        self.entry(account)?
            .set_password(secret)
            .map_err(map_keyring_error)
    }

    fn delete(&self, account: &str) -> Result<(), SecretStoreError> {
        match self.entry(account)?.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(error) => Err(map_keyring_error(error)),
        }
    }
}

fn validate_account(account: &str) -> Result<(), SecretStoreError> {
    if account.is_empty()
        || account.len() > 512
        || account.trim() != account
        || account.chars().any(char::is_control)
    {
        return Err(SecretStoreError::InvalidInput);
    }
    Ok(())
}

fn map_keyring_error(error: keyring::Error) -> SecretStoreError {
    match error {
        keyring::Error::NoDefaultStore | keyring::Error::NoStorageAccess(_) => {
            SecretStoreError::Unavailable
        }
        keyring::Error::Invalid(_, _) | keyring::Error::TooLong(_, _) => {
            SecretStoreError::InvalidInput
        }
        _ => SecretStoreError::Backend,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn account_labels_reject_unsafe_values() {
        assert_eq!(validate_account(""), Err(SecretStoreError::InvalidInput));
        assert_eq!(
            validate_account(" refresh-token"),
            Err(SecretStoreError::InvalidInput)
        );
        assert_eq!(
            validate_account("refresh-token\nuser"),
            Err(SecretStoreError::InvalidInput)
        );
        assert_eq!(validate_account("refresh-token:user-42"), Ok(()));
    }

    #[test]
    fn public_errors_never_include_backend_details() {
        assert_eq!(
            SecretStoreError::Unavailable.public_code(),
            "SECURE_STORE_UNAVAILABLE"
        );
        assert_eq!(
            SecretStoreError::InvalidInput.public_code(),
            "SECURE_STORE_INVALID_INPUT"
        );
        assert_eq!(
            SecretStoreError::Backend.public_code(),
            "SECURE_STORE_ERROR"
        );
    }

    #[test]
    fn unavailable_store_fails_closed() {
        let store = UnavailableSecretStore;
        assert!(!store.is_supported());
        assert_eq!(
            store.get("refresh-token:user-42"),
            Err(SecretStoreError::Unavailable)
        );
        assert_eq!(
            store.set("refresh-token:user-42", "secret"),
            Err(SecretStoreError::Unavailable)
        );
        assert_eq!(
            store.delete("refresh-token:user-42"),
            Err(SecretStoreError::Unavailable)
        );
    }
}
