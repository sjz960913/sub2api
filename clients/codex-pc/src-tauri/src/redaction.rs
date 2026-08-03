//! Field-level log redaction. Never pass raw tokens, prompts or local paths to tracing.

pub fn redact_secret(value: &str) -> String {
    if value.is_empty() {
        return String::new();
    }
    let suffix: String = value.chars().rev().take(4).collect::<String>().chars().rev().collect();
    format!("••••{suffix}")
}

#[cfg(test)]
mod tests {
    use super::redact_secret;

    #[test]
    fn keeps_only_the_last_four_characters() {
        assert_eq!(redact_secret("Bearer abcdefgh"), "••••efgh");
        assert_eq!(redact_secret(""), "");
    }
}
