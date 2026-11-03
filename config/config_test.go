package config

import (
	"strings"
	"testing"
)

const testSecret = "0123456789abcdef0123456789abcdef"
const testEncKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// validEnv returns the required variables plus the given overrides.
func validEnv(overrides map[string]string) map[string]string {
	m := map[string]string{"DELSIGN_JWT_SECRET": testSecret, "DELSIGN_KEY_ENC_KEY": testEncKey}
	for k, v := range overrides {
		m[k] = v
	}
	return m
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(envOf(validEnv(nil)))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Port != "3010" || cfg.DBPath != "delsign.db" || cfg.RPCURL != "http://localhost:8545" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.ChainID.Int64() != 31337 {
		t.Errorf("chain id = %s, want 31337", cfg.ChainID)
	}
	if len(cfg.KeyEncKey) != 32 || cfg.KeyEncKey[31] != 0x1f {
		t.Errorf("key enc key = %x, want the decoded test key", cfg.KeyEncKey)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(envOf(validEnv(map[string]string{
		"DELSIGN_PORT":     "8080",
		"DELSIGN_DB_PATH":  "/tmp/x.db",
		"DELSIGN_RPC_URL":  "http://node:8545",
		"DELSIGN_CHAIN_ID": "1337",
	})))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Port != "8080" || cfg.DBPath != "/tmp/x.db" || cfg.RPCURL != "http://node:8545" || cfg.ChainID.Int64() != 1337 {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"missing secret", map[string]string{"DELSIGN_KEY_ENC_KEY": testEncKey}, "DELSIGN_JWT_SECRET is required"},
		{"short secret", validEnv(map[string]string{"DELSIGN_JWT_SECRET": "secret"}), "at least 32 bytes"},
		{"bad port", validEnv(map[string]string{"DELSIGN_PORT": "http"}), "DELSIGN_PORT"},
		{"port out of range", validEnv(map[string]string{"DELSIGN_PORT": "70000"}), "DELSIGN_PORT"},
		{"bad chain id", validEnv(map[string]string{"DELSIGN_CHAIN_ID": "abc"}), "DELSIGN_CHAIN_ID"},
		{"zero chain id", validEnv(map[string]string{"DELSIGN_CHAIN_ID": "0"}), "DELSIGN_CHAIN_ID"},
		{"missing enc key", map[string]string{"DELSIGN_JWT_SECRET": testSecret}, "DELSIGN_KEY_ENC_KEY is required"},
		{"enc key not hex", validEnv(map[string]string{"DELSIGN_KEY_ENC_KEY": strings.Repeat("zz", 32)}), "DELSIGN_KEY_ENC_KEY must be 64 hex"},
		{"enc key too short", validEnv(map[string]string{"DELSIGN_KEY_ENC_KEY": testEncKey[:62]}), "DELSIGN_KEY_ENC_KEY must be 64 hex"},
		{"enc key too long", validEnv(map[string]string{"DELSIGN_KEY_ENC_KEY": testEncKey + "00"}), "DELSIGN_KEY_ENC_KEY must be 64 hex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(envOf(tt.env))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
