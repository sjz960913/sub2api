use crate::panel_auth::PanelConnectionContext;
use crate::protocol::collaboration_wire::{
    DeviceCapabilities, RegisterDeviceRequest, RegisterDeviceResult,
};
use crate::secret_store::{SecretStore, SecretStoreError};
use reqwest::blocking::Client;
use reqwest::redirect::Policy;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::fmt::Write as _;
use std::io::Read;
use std::sync::Arc;
use std::time::Duration;
use url::Url;
use uuid::Uuid;

const INSTALLATION_ID_ACCOUNT: &str = "installation-id";
const MAX_RESPONSE_BYTES: u64 = 1024 * 1024;

#[derive(Clone, Debug, serde::Serialize, PartialEq, Eq)]
pub struct RegisteredDevice {
    pub device_id: String,
    pub heartbeat_interval_seconds: i64,
    pub event_protocol_version: i64,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum DeviceRegistrationError {
    Unauthorized,
    Forbidden,
    Disabled,
    Network,
    InvalidResponse,
    ResponseTooLarge,
    CorruptInstallationIdentity,
    SecureStore(SecretStoreError),
}

impl DeviceRegistrationError {
    pub fn public_code(self) -> &'static str {
        match self {
            Self::Unauthorized => "COLLAB_UNAUTHORIZED",
            Self::Forbidden => "COLLAB_FORBIDDEN",
            Self::Disabled => "COLLABORATION_DISABLED",
            Self::Network => "COLLAB_NETWORK_ERROR",
            Self::InvalidResponse => "COLLAB_INVALID_RESPONSE",
            Self::ResponseTooLarge => "COLLAB_RESPONSE_TOO_LARGE",
            Self::CorruptInstallationIdentity => "COLLAB_INSTALLATION_ID_CORRUPT",
            Self::SecureStore(error) => error.public_code(),
        }
    }
}

pub struct DeviceRegistrar {
    client: Client,
    store: Arc<dyn SecretStore>,
}

#[derive(Deserialize)]
struct Envelope<T> {
    code: i64,
    data: Option<T>,
}

impl DeviceRegistrar {
    pub fn new(store: Arc<dyn SecretStore>) -> Result<Self, DeviceRegistrationError> {
        let client = Client::builder()
            .connect_timeout(Duration::from_secs(5))
            .timeout(Duration::from_secs(15))
            .redirect(Policy::none())
            .user_agent("Sub2API-Codex-PC/0.1")
            .build()
            .map_err(|_| DeviceRegistrationError::Network)?;
        Ok(Self { client, store })
    }

    pub fn register(
        &self,
        context: &PanelConnectionContext,
    ) -> Result<RegisteredDevice, DeviceRegistrationError> {
        let installation_id_hash = self.installation_id_hash()?;
        let endpoint = Url::parse(&context.site_url)
            .and_then(|url| url.join("api/v1/collaboration/devices/register"))
            .map_err(|_| DeviceRegistrationError::InvalidResponse)?;
        let request = RegisterDeviceRequest {
            installation_id_hash,
            name: "Codex PC".to_owned(),
            platform: platform_name().to_owned(),
            platform_version: None,
            companion_version: env!("CARGO_PKG_VERSION").to_owned(),
            codex_version: None,
            protocol_version: 1,
            capabilities: DeviceCapabilities {
                app_server: true,
                thread_read: true,
                thread_write: true,
                image_input: false,
            },
        };
        let response = self
            .client
            .post(endpoint)
            .bearer_auth(context.access_token.as_str())
            .json(&request)
            .send()
            .map_err(|_| DeviceRegistrationError::Network)?;
        if !response.status().is_success() {
            return Err(match response.status().as_u16() {
                401 => DeviceRegistrationError::Unauthorized,
                403 => DeviceRegistrationError::Forbidden,
                503 => DeviceRegistrationError::Disabled,
                _ => DeviceRegistrationError::InvalidResponse,
            });
        }
        if response
            .content_length()
            .is_some_and(|length| length > MAX_RESPONSE_BYTES)
        {
            return Err(DeviceRegistrationError::ResponseTooLarge);
        }
        let mut body = Vec::new();
        response
            .take(MAX_RESPONSE_BYTES + 1)
            .read_to_end(&mut body)
            .map_err(|_| DeviceRegistrationError::Network)?;
        if body.len() as u64 > MAX_RESPONSE_BYTES {
            return Err(DeviceRegistrationError::ResponseTooLarge);
        }
        let envelope: Envelope<RegisterDeviceResult> =
            serde_json::from_slice(&body).map_err(|_| DeviceRegistrationError::InvalidResponse)?;
        let data = envelope
            .data
            .filter(|_| envelope.code == 0)
            .ok_or(DeviceRegistrationError::InvalidResponse)?;
        if Uuid::parse_str(&data.device_id).is_err()
            || data.heartbeat_interval_seconds < 5
            || data.event_protocol_version != 1
        {
            return Err(DeviceRegistrationError::InvalidResponse);
        }
        Ok(RegisteredDevice {
            device_id: data.device_id,
            heartbeat_interval_seconds: data.heartbeat_interval_seconds,
            event_protocol_version: data.event_protocol_version,
        })
    }

    fn installation_id_hash(&self) -> Result<String, DeviceRegistrationError> {
        let installation_id = match self
            .store
            .get(INSTALLATION_ID_ACCOUNT)
            .map_err(DeviceRegistrationError::SecureStore)?
        {
            Some(value) => {
                Uuid::parse_str(&value)
                    .map_err(|_| DeviceRegistrationError::CorruptInstallationIdentity)?;
                value
            }
            None => {
                let value = Uuid::new_v4().to_string();
                self.store
                    .set(INSTALLATION_ID_ACCOUNT, &value)
                    .map_err(DeviceRegistrationError::SecureStore)?;
                value
            }
        };
        Ok(hash_installation_id(&installation_id))
    }
}

fn platform_name() -> &'static str {
    match std::env::consts::OS {
        "macos" => "macos",
        "windows" => "windows",
        _ => "linux",
    }
}

fn hash_installation_id(installation_id: &str) -> String {
    let digest = Sha256::digest(installation_id.as_bytes());
    let mut output = String::with_capacity(7 + digest.len() * 2);
    output.push_str("sha256:");
    for byte in digest {
        let _ = write!(output, "{byte:02x}");
    }
    output
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;
    use std::sync::Mutex;

    #[derive(Default)]
    struct MemoryStore {
        values: Mutex<HashMap<String, String>>,
    }

    impl SecretStore for MemoryStore {
        fn is_supported(&self) -> bool {
            true
        }

        fn get(&self, account: &str) -> Result<Option<String>, SecretStoreError> {
            Ok(self.values.lock().unwrap().get(account).cloned())
        }

        fn set(&self, account: &str, secret: &str) -> Result<(), SecretStoreError> {
            self.values
                .lock()
                .unwrap()
                .insert(account.to_owned(), secret.to_owned());
            Ok(())
        }

        fn delete(&self, account: &str) -> Result<(), SecretStoreError> {
            self.values.lock().unwrap().remove(account);
            Ok(())
        }
    }

    #[test]
    fn installation_hash_matches_wire_contract_and_is_stable() {
        let store = Arc::new(MemoryStore::default());
        let registrar = DeviceRegistrar::new(store).unwrap();
        let first = registrar.installation_id_hash().unwrap();
        let second = registrar.installation_id_hash().unwrap();
        assert_eq!(first, second);
        assert!(first.starts_with("sha256:"));
        assert_eq!(first.len(), 7 + 64);
        assert!(first[7..].bytes().all(|byte| byte.is_ascii_hexdigit()));
    }

    #[test]
    fn corrupt_installation_identity_fails_closed() {
        let store = Arc::new(MemoryStore::default());
        store.set(INSTALLATION_ID_ACCOUNT, "not-a-uuid").unwrap();
        let registrar = DeviceRegistrar::new(store).unwrap();
        assert_eq!(
            registrar.installation_id_hash(),
            Err(DeviceRegistrationError::CorruptInstallationIdentity)
        );
    }

    #[test]
    fn registration_request_contains_capabilities_but_no_secrets() {
        let request = RegisterDeviceRequest {
            installation_id_hash: hash_installation_id("fixture-installation"),
            name: "Codex PC".to_owned(),
            platform: platform_name().to_owned(),
            platform_version: None,
            companion_version: "0.1.0".to_owned(),
            codex_version: None,
            protocol_version: 1,
            capabilities: DeviceCapabilities {
                app_server: true,
                thread_read: true,
                thread_write: true,
                image_input: false,
            },
        };
        let json = serde_json::to_string(&request).unwrap();
        for forbidden in ["access_token", "refresh_token", "password", "api_key"] {
            assert!(!json.contains(forbidden));
        }
    }
}
