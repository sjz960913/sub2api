package migrations

import (
	"strings"
	"testing"
)

func TestForkDefaultCodexFingerprintMigrationIsScopedAndIdempotent(t *testing.T) {
	sqlBytes, err := FS.ReadFile("226a_fork_default_codex_fingerprint_session.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(sqlBytes)
	for _, fragment := range []string{
		"platform = 'openai'",
		"type = 'oauth'",
		"COALESCE(extra->>'codex_fingerprint_mode', '') <> 'off'",
		"gen_random_uuid()",
		"extra->>'codex_fingerprint_seed' IS NULL",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing guard %q", fragment)
		}
	}
}
