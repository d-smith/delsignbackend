package config

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
)

// Minimum HS256 key length, per RFC 7518 section 3.2 (256 bits)
const minJWTSecretLen = 32

type Config struct {
	Port      string
	DBPath    string
	RPCURL    string
	ChainID   *big.Int
	JWTSecret string
}

// Load reads the configuration from DELSIGN_* environment variables, applying
// development defaults for everything except secrets.
func Load() (*Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (*Config, error) {
	withDefault := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}

	cfg := &Config{
		Port:      withDefault("DELSIGN_PORT", "3010"),
		DBPath:    withDefault("DELSIGN_DB_PATH", "delsign.db"),
		RPCURL:    withDefault("DELSIGN_RPC_URL", "http://localhost:8545"),
		JWTSecret: getenv("DELSIGN_JWT_SECRET"),
	}

	port, err := strconv.Atoi(cfg.Port)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("DELSIGN_PORT must be a port number, got %q", cfg.Port)
	}

	chainID := withDefault("DELSIGN_CHAIN_ID", "31337")
	id, ok := new(big.Int).SetString(chainID, 10)
	if !ok || id.Sign() <= 0 {
		return nil, fmt.Errorf("DELSIGN_CHAIN_ID must be a positive integer, got %q", chainID)
	}
	cfg.ChainID = id

	if cfg.JWTSecret == "" {
		return nil, errors.New("DELSIGN_JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < minJWTSecretLen {
		return nil, fmt.Errorf("DELSIGN_JWT_SECRET must be at least %d bytes", minJWTSecretLen)
	}

	return cfg, nil
}
