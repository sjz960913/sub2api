use crate::secret_store::{SecretStore, SecretStoreError};
use reqwest::blocking::{Client, RequestBuilder};
use reqwest::redirect::Policy;
use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::fmt::Write as _;
use std::io::Read;
use std::net::IpAddr;
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use url::Url;
use zeroize::Zeroizing;

const MAX_RESPONSE_BYTES: u64 = 1024 * 1024;
const MAX_TOKEN_BYTES: usize = 8192;

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct PublicSession {
    pub site_url: String,
    pub user_id: i64,
    pub email: String,
    pub role: String,
    pub expires_at_epoch_seconds: u64,
}

#[derive(Debug, Serialize, PartialEq, Eq)]
#[serde(tag = "status", rename_all = "snake_case")]
pub enum LoginResult {
    Authenticated { session: PublicSession },
    RequiresTwoFactor { email_masked: String },
}

#[derive(Debug, Serialize, PartialEq, Eq)]
pub struct PanelAuthStatus {
    pub authenticated: bool,
    pub requires_two_factor: bool,
    pub session: Option<PublicSession>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PanelAuthError {
    InvalidSite,
    InsecureSite,
    InvalidEmail,
    InvalidPassword,
    InvalidTurnstileToken,
    InvalidTwoFactorCode,
    NoPendingTwoFactor,
    SessionNotFound,
    BadRequest,
    Unauthorized,
    Forbidden,
    RateLimited,
    Server,
    Network,
    InvalidResponse,
    ResponseTooLarge,
    SecureStore(SecretStoreError),
}

impl PanelAuthError {
    pub fn public_code(self) -> &'static str {
        match self {
            Self::InvalidSite => "PANEL_INVALID_SITE",
            Self::InsecureSite => "PANEL_INSECURE_SITE",
            Self::InvalidEmail => "PANEL_INVALID_EMAIL",
            Self::InvalidPassword => "PANEL_INVALID_PASSWORD",
            Self::InvalidTurnstileToken => "PANEL_INVALID_TURNSTILE_TOKEN",
            Self::InvalidTwoFactorCode => "PANEL_INVALID_TWO_FACTOR_CODE",
            Self::NoPendingTwoFactor => "PANEL_NO_PENDING_TWO_FACTOR",
            Self::SessionNotFound => "PANEL_SESSION_NOT_FOUND",
            Self::BadRequest => "PANEL_BAD_REQUEST",
            Self::Unauthorized => "PANEL_UNAUTHORIZED",
            Self::Forbidden => "PANEL_FORBIDDEN",
            Self::RateLimited => "PANEL_RATE_LIMITED",
            Self::Server => "PANEL_SERVER_ERROR",
            Self::Network => "PANEL_NETWORK_ERROR",
            Self::InvalidResponse => "PANEL_INVALID_RESPONSE",
            Self::ResponseTooLarge => "PANEL_RESPONSE_TOO_LARGE",
            Self::SecureStore(error) => error.public_code(),
        }
    }
}

pub struct PanelAuthService {
    client: Client,
    store: Arc<dyn SecretStore>,
    operation: Mutex<()>,
    state: Mutex<AuthState>,
}

#[derive(Default)]
struct AuthState {
    pending_two_factor: Option<PendingTwoFactor>,
    session: Option<PrivateSession>,
}

struct PendingTwoFactor {
    site_url: String,
    email: String,
    temp_token: Zeroizing<String>,
}

struct PrivateSession {
    public: PublicSession,
    access_token: Zeroizing<String>,
    refresh_account: String,
}

#[derive(Deserialize)]
struct Envelope<T> {
    code: i64,
    data: Option<T>,
}

#[derive(Deserialize)]
struct LoginData {
    access_token: Option<String>,
    refresh_token: Option<String>,
    expires_in: Option<u64>,
    user: Option<PanelUser>,
    requires_2fa: Option<bool>,
    temp_token: Option<String>,
    user_email_masked: Option<String>,
}

#[derive(Deserialize)]
struct RefreshData {
    access_token: String,
    refresh_token: String,
    expires_in: u64,
}

#[derive(Deserialize)]
struct PanelUser {
    id: i64,
    email: String,
    role: String,
}

#[derive(Serialize)]
struct LoginRequest<'a> {
    email: &'a str,
    password: &'a str,
    turnstile_token: &'a str,
}

#[derive(Serialize)]
struct LoginTwoFactorRequest<'a> {
    temp_token: &'a str,
    totp_code: &'a str,
}

#[derive(Serialize)]
struct RefreshRequest<'a> {
    refresh_token: &'a str,
}

#[derive(Serialize)]
struct LogoutRequest<'a> {
    refresh_token: &'a str,
}

impl PanelAuthService {
    pub fn new(store: Arc<dyn SecretStore>) -> Result<Self, PanelAuthError> {
        let client = Client::builder()
            .connect_timeout(Duration::from_secs(5))
            .timeout(Duration::from_secs(15))
            .redirect(Policy::none())
            .user_agent("Sub2API-Codex-PC/0.1")
            .build()
            .map_err(|_| PanelAuthError::Network)?;
        Ok(Self {
            client,
            store,
            operation: Mutex::new(()),
            state: Mutex::new(AuthState::default()),
        })
    }

    pub fn secure_store_supported(&self) -> bool {
        self.store.is_supported()
    }

    pub fn status(&self) -> PanelAuthStatus {
        let state = self.state.lock().unwrap_or_else(|error| error.into_inner());
        PanelAuthStatus {
            authenticated: state.session.is_some(),
            requires_two_factor: state.pending_two_factor.is_some(),
            session: state.session.as_ref().map(|session| session.public.clone()),
        }
    }

    pub fn login(
        &self,
        site_url: String,
        email: String,
        password: String,
        turnstile_token: Option<String>,
    ) -> Result<LoginResult, PanelAuthError> {
        let _operation = self
            .operation
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let site_url = normalize_site_url(&site_url)?;
        let email = normalize_email(&email)?;
        if password.is_empty() || password.len() > 4096 {
            return Err(PanelAuthError::InvalidPassword);
        }
        let password = Zeroizing::new(password);
        let turnstile_token = Zeroizing::new(turnstile_token.unwrap_or_default());
        if turnstile_token.len() > MAX_TOKEN_BYTES || turnstile_token.chars().any(char::is_control)
        {
            return Err(PanelAuthError::InvalidTurnstileToken);
        }

        let endpoint = endpoint(&site_url, "api/v1/auth/login")?;
        let data: LoginData = self.request(self.client.post(endpoint).json(&LoginRequest {
            email: &email,
            password: password.as_str(),
            turnstile_token: turnstile_token.as_str(),
        }))?;

        if data.requires_2fa.unwrap_or(false) {
            let temp_token = data.temp_token.ok_or(PanelAuthError::InvalidResponse)?;
            if temp_token.is_empty() || temp_token.len() > MAX_TOKEN_BYTES {
                return Err(PanelAuthError::InvalidResponse);
            }
            let email_masked = data.user_email_masked.unwrap_or_default();
            let mut state = self.state.lock().unwrap_or_else(|error| error.into_inner());
            state.session = None;
            state.pending_two_factor = Some(PendingTwoFactor {
                site_url,
                email,
                temp_token: Zeroizing::new(temp_token),
            });
            return Ok(LoginResult::RequiresTwoFactor { email_masked });
        }

        let session = self.finish_login(site_url, email, data)?;
        Ok(LoginResult::Authenticated { session })
    }

    pub fn complete_two_factor(&self, code: String) -> Result<PublicSession, PanelAuthError> {
        let _operation = self
            .operation
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        if code.len() != 6 || !code.bytes().all(|byte| byte.is_ascii_digit()) {
            return Err(PanelAuthError::InvalidTwoFactorCode);
        }
        let code = Zeroizing::new(code);
        let (site_url, email, temp_token) = {
            let state = self.state.lock().unwrap_or_else(|error| error.into_inner());
            let pending = state
                .pending_two_factor
                .as_ref()
                .ok_or(PanelAuthError::NoPendingTwoFactor)?;
            (
                pending.site_url.clone(),
                pending.email.clone(),
                Zeroizing::new(pending.temp_token.to_string()),
            )
        };
        let endpoint = endpoint(&site_url, "api/v1/auth/login/2fa")?;
        let data: LoginData =
            self.request(self.client.post(endpoint).json(&LoginTwoFactorRequest {
                temp_token: temp_token.as_str(),
                totp_code: code.as_str(),
            }))?;
        let session = self.finish_login(site_url, email, data)?;
        Ok(session)
    }

    pub fn restore(
        &self,
        site_url: String,
        email: String,
    ) -> Result<PublicSession, PanelAuthError> {
        let _operation = self
            .operation
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let site_url = normalize_site_url(&site_url)?;
        let email = normalize_email(&email)?;
        let account = refresh_account(&site_url, &email);
        let refresh_token = self
            .store
            .get(&account)
            .map_err(PanelAuthError::SecureStore)?
            .ok_or(PanelAuthError::SessionNotFound)?;
        let refresh_token = Zeroizing::new(refresh_token);
        self.refresh_with_token(site_url, email, account, refresh_token)
    }

    pub fn refresh(&self) -> Result<PublicSession, PanelAuthError> {
        let _operation = self
            .operation
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let (site_url, email, account) = {
            let state = self.state.lock().unwrap_or_else(|error| error.into_inner());
            let session = state
                .session
                .as_ref()
                .ok_or(PanelAuthError::SessionNotFound)?;
            (
                session.public.site_url.clone(),
                session.public.email.clone(),
                session.refresh_account.clone(),
            )
        };
        let refresh_token = self
            .store
            .get(&account)
            .map_err(PanelAuthError::SecureStore)?
            .ok_or(PanelAuthError::SessionNotFound)?;
        self.refresh_with_token(site_url, email, account, Zeroizing::new(refresh_token))
    }

    pub fn logout(&self) -> Result<(), PanelAuthError> {
        let _operation = self
            .operation
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let (site_url, account) = {
            let state = self.state.lock().unwrap_or_else(|error| error.into_inner());
            let session = state
                .session
                .as_ref()
                .ok_or(PanelAuthError::SessionNotFound)?;
            (
                session.public.site_url.clone(),
                session.refresh_account.clone(),
            )
        };
        let refresh_token = self
            .store
            .get(&account)
            .map_err(PanelAuthError::SecureStore)?;
        if let Some(refresh_token) = refresh_token {
            let refresh_token = Zeroizing::new(refresh_token);
            if let Ok(endpoint) = endpoint(&site_url, "api/v1/auth/logout") {
                let _: Result<serde_json::Value, _> =
                    self.request(self.client.post(endpoint).json(&LogoutRequest {
                        refresh_token: refresh_token.as_str(),
                    }));
            }
        }
        self.store
            .delete(&account)
            .map_err(PanelAuthError::SecureStore)?;
        let mut state = self.state.lock().unwrap_or_else(|error| error.into_inner());
        state.session = None;
        state.pending_two_factor = None;
        Ok(())
    }

    #[allow(dead_code)]
    pub fn access_token(&self) -> Result<Zeroizing<String>, PanelAuthError> {
        let state = self.state.lock().unwrap_or_else(|error| error.into_inner());
        let session = state
            .session
            .as_ref()
            .ok_or(PanelAuthError::SessionNotFound)?;
        Ok(Zeroizing::new(session.access_token.to_string()))
    }

    fn finish_login(
        &self,
        site_url: String,
        expected_email: String,
        data: LoginData,
    ) -> Result<PublicSession, PanelAuthError> {
        let access_token = data.access_token.ok_or(PanelAuthError::InvalidResponse)?;
        let refresh_token = data.refresh_token.ok_or(PanelAuthError::InvalidResponse)?;
        validate_token(&access_token)?;
        validate_token(&refresh_token)?;
        let user = data.user.ok_or(PanelAuthError::InvalidResponse)?;
        let user_email = normalize_email(&user.email)?;
        if user_email != expected_email || user.id <= 0 || user.role.is_empty() {
            return Err(PanelAuthError::InvalidResponse);
        }
        let account = refresh_account(&site_url, &expected_email);
        self.store
            .set(&account, &refresh_token)
            .map_err(PanelAuthError::SecureStore)?;
        let public = PublicSession {
            site_url,
            user_id: user.id,
            email: expected_email,
            role: user.role,
            expires_at_epoch_seconds: expires_at(data.expires_in.unwrap_or_default()),
        };
        let mut state = self.state.lock().unwrap_or_else(|error| error.into_inner());
        state.pending_two_factor = None;
        state.session = Some(PrivateSession {
            public: public.clone(),
            access_token: Zeroizing::new(access_token),
            refresh_account: account,
        });
        Ok(public)
    }

    fn refresh_with_token(
        &self,
        site_url: String,
        expected_email: String,
        account: String,
        refresh_token: Zeroizing<String>,
    ) -> Result<PublicSession, PanelAuthError> {
        let refresh_endpoint = endpoint(&site_url, "api/v1/auth/refresh")?;
        let data: RefreshData =
            self.request(self.client.post(refresh_endpoint).json(&RefreshRequest {
                refresh_token: refresh_token.as_str(),
            }))?;
        validate_token(&data.access_token)?;
        validate_token(&data.refresh_token)?;
        self.store
            .set(&account, &data.refresh_token)
            .map_err(PanelAuthError::SecureStore)?;

        let profile_endpoint = endpoint(&site_url, "api/v1/auth/me")?;
        let user: PanelUser = self.request(
            self.client
                .get(profile_endpoint)
                .bearer_auth(&data.access_token),
        )?;
        let user_email = normalize_email(&user.email)?;
        if user_email != expected_email || user.id <= 0 || user.role.is_empty() {
            return Err(PanelAuthError::InvalidResponse);
        }
        let public = PublicSession {
            site_url,
            user_id: user.id,
            email: expected_email,
            role: user.role,
            expires_at_epoch_seconds: expires_at(data.expires_in),
        };
        let mut state = self.state.lock().unwrap_or_else(|error| error.into_inner());
        state.pending_two_factor = None;
        state.session = Some(PrivateSession {
            public: public.clone(),
            access_token: Zeroizing::new(data.access_token),
            refresh_account: account,
        });
        Ok(public)
    }

    fn request<T: DeserializeOwned>(&self, request: RequestBuilder) -> Result<T, PanelAuthError> {
        let mut response = request.send().map_err(|_| PanelAuthError::Network)?;
        let status = response.status();
        if !status.is_success() {
            return Err(match status.as_u16() {
                400 => PanelAuthError::BadRequest,
                401 => PanelAuthError::Unauthorized,
                403 => PanelAuthError::Forbidden,
                429 => PanelAuthError::RateLimited,
                500..=599 => PanelAuthError::Server,
                _ => PanelAuthError::InvalidResponse,
            });
        }
        if response
            .content_length()
            .is_some_and(|length| length > MAX_RESPONSE_BYTES)
        {
            return Err(PanelAuthError::ResponseTooLarge);
        }
        let mut body = Vec::new();
        response
            .take(MAX_RESPONSE_BYTES + 1)
            .read_to_end(&mut body)
            .map_err(|_| PanelAuthError::Network)?;
        if body.len() as u64 > MAX_RESPONSE_BYTES {
            return Err(PanelAuthError::ResponseTooLarge);
        }
        let envelope: Envelope<T> =
            serde_json::from_slice(&body).map_err(|_| PanelAuthError::InvalidResponse)?;
        if envelope.code != 0 {
            return Err(PanelAuthError::InvalidResponse);
        }
        envelope.data.ok_or(PanelAuthError::InvalidResponse)
    }
}

fn normalize_site_url(raw: &str) -> Result<String, PanelAuthError> {
    let mut url = Url::parse(raw.trim()).map_err(|_| PanelAuthError::InvalidSite)?;
    if !url.username().is_empty()
        || url.password().is_some()
        || url.query().is_some()
        || url.fragment().is_some()
        || url.host_str().is_none()
    {
        return Err(PanelAuthError::InvalidSite);
    }
    match url.scheme() {
        "https" => {}
        "http" if is_loopback_host(url.host_str().unwrap_or_default()) => {}
        "http" => return Err(PanelAuthError::InsecureSite),
        _ => return Err(PanelAuthError::InvalidSite),
    }
    if !url.path().ends_with('/') {
        let path = format!("{}/", url.path());
        url.set_path(&path);
    }
    Ok(url.to_string())
}

fn is_loopback_host(host: &str) -> bool {
    host.eq_ignore_ascii_case("localhost")
        || host
            .parse::<IpAddr>()
            .map(|address| address.is_loopback())
            .unwrap_or(false)
}

fn normalize_email(raw: &str) -> Result<String, PanelAuthError> {
    let email = raw.trim().to_lowercase();
    if email.is_empty()
        || email.len() > 254
        || email.chars().any(char::is_control)
        || !email.contains('@')
    {
        return Err(PanelAuthError::InvalidEmail);
    }
    Ok(email)
}

fn endpoint(site_url: &str, path: &str) -> Result<Url, PanelAuthError> {
    Url::parse(site_url)
        .and_then(|url| url.join(path))
        .map_err(|_| PanelAuthError::InvalidSite)
}

fn validate_token(token: &str) -> Result<(), PanelAuthError> {
    if token.is_empty() || token.len() > MAX_TOKEN_BYTES || token.chars().any(char::is_control) {
        return Err(PanelAuthError::InvalidResponse);
    }
    Ok(())
}

fn refresh_account(site_url: &str, email: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(b"sub2api-codex-pc-refresh\0");
    hasher.update(site_url.as_bytes());
    hasher.update(b"\0");
    hasher.update(email.as_bytes());
    let digest = hasher.finalize();
    let mut output = String::with_capacity(14 + digest.len() * 2);
    output.push_str("refresh-token:");
    for byte in digest {
        let _ = write!(output, "{byte:02x}");
    }
    output
}

fn expires_at(expires_in: u64) -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
        .saturating_add(expires_in)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

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
    fn site_url_requires_tls_except_for_loopback() {
        assert_eq!(
            normalize_site_url("http://example.com"),
            Err(PanelAuthError::InsecureSite)
        );
        assert_eq!(
            normalize_site_url("https://panel.example.com/base"),
            Ok("https://panel.example.com/base/".to_owned())
        );
        assert_eq!(
            normalize_site_url("http://127.0.0.1:8080"),
            Ok("http://127.0.0.1:8080/".to_owned())
        );
    }

    #[test]
    fn refresh_account_does_not_expose_site_or_email() {
        let account = refresh_account("https://panel.example.com/", "user@example.com");
        assert!(account.starts_with("refresh-token:"));
        assert!(!account.contains("panel.example.com"));
        assert!(!account.contains("user@example.com"));
        assert_eq!(account.len(), "refresh-token:".len() + 64);
    }

    #[test]
    fn public_status_serialization_has_no_tokens_or_billing() {
        let status = PanelAuthStatus {
            authenticated: true,
            requires_two_factor: false,
            session: Some(PublicSession {
                site_url: "https://panel.example.com/".to_owned(),
                user_id: 42,
                email: "user@example.com".to_owned(),
                role: "user".to_owned(),
                expires_at_epoch_seconds: 123,
            }),
        };
        let json = serde_json::to_string(&status).unwrap();
        for forbidden in [
            "access_token",
            "refresh_token",
            "password",
            "balance",
            "charge",
            "refund",
            "approval",
        ] {
            assert!(!json.contains(forbidden));
        }
    }

    #[test]
    fn memory_store_exercises_secret_store_contract() {
        let store = MemoryStore::default();
        assert_eq!(store.get("account"), Ok(None));
        store.set("account", "secret").unwrap();
        assert_eq!(store.get("account"), Ok(Some("secret".to_owned())));
        store.delete("account").unwrap();
        assert_eq!(store.get("account"), Ok(None));
    }
}
