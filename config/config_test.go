package config

import (
	"strings"
	"testing"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(envOf(map[string]string{"DELSIGN_JWT_SECRET": testSecret}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Port != "3010" || cfg.DBPath != "delsign.db" || cfg.RPCURL != "http://localhost:8545" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.ChainID.Int64() != 31337 {
		t.Errorf("chain id = %s, want 31337", cfg.ChainID)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(envOf(map[string]string{
		"DELSIGN_JWT_SECRET": testSecret,
		"DELSIGN_PORT":       "8080",
		"DELSIGN_DB_PATH":    "/tmp/x.db",
		"DELSIGN_RPC_URL":    "http://node:8545",
		"DELSIGN_CHAIN_ID":   "1337",
	}))
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
		{"missing secret", map[string]string{}, "DELSIGN_JWT_SECRET is required"},
		{"short secret", map[string]string{"DELSIGN_JWT_SECRET": "secret"}, "at least 32 bytes"},
		{"bad port", map[string]string{"DELSIGN_JWT_SECRET": testSecret, "DELSIGN_PORT": "http"}, "DELSIGN_PORT"},
		{"port out of range", map[string]string{"DELSIGN_JWT_SECRET": testSecret, "DELSIGN_PORT": "70000"}, "DELSIGN_PORT"},
		{"bad chain id", map[string]string{"DELSIGN_JWT_SECRET": testSecret, "DELSIGN_CHAIN_ID": "abc"}, "DELSIGN_CHAIN_ID"},
		{"zero chain id", map[string]string{"DELSIGN_JWT_SECRET": testSecret, "DELSIGN_CHAIN_ID": "0"}, "DELSIGN_CHAIN_ID"},
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
