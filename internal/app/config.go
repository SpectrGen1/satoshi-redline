package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Bitcoin      BitcoinConfig        `json:"bitcoin"`
	API          APIConfig            `json:"api"`
	DatabasePath string               `json:"database_path"`
	RPCTimeoutMS int                  `json:"rpc_timeout_ms"`
	Sync         SyncThresholds       `json:"synchronization"`
	Filesystem   FilesystemThresholds `json:"filesystem"`
}
type BitcoinConfig struct {
	RPCEndpoint   string `json:"rpc_endpoint"`
	CookieFile    string `json:"cookie_file"`
	CookieEnv     string `json:"cookie_env"`
	DataDirectory string `json:"data_directory"`
}
type APIConfig struct {
	ListenAddress string `json:"listen_address"`
}
type SyncThresholds struct {
	MaxTipAgeSeconds        int64   `json:"max_tip_age_seconds"`
	MinVerificationProgress float64 `json:"min_verification_progress"`
	MaxHeaderLag            int64   `json:"max_header_lag"`
}
type FilesystemThresholds struct {
	WarningAvailablePercent float64 `json:"warning_available_percent"`
	FailureAvailablePercent float64 `json:"failure_available_percent"`
}

func DefaultConfig() Config {
	return Config{API: APIConfig{"127.0.0.1:8383"}, DatabasePath: "redline.db", RPCTimeoutMS: 5000, Sync: SyncThresholds{7200, .999, 2}, Filesystem: FilesystemThresholds{15, 5}}
}
func Load(path string) (Config, error) {
	c := DefaultConfig()
	if path != "" {
		b, e := os.ReadFile(path)
		if e != nil {
			return c, e
		}
		if e = json.Unmarshal(b, &c); e != nil {
			return c, fmt.Errorf("invalid configuration: %w", e)
		}
	}
	if v := os.Getenv("REDLINE_RPC_ENDPOINT"); v != "" {
		c.Bitcoin.RPCEndpoint = v
	}
	if v := os.Getenv("REDLINE_DATA_DIRECTORY"); v != "" {
		c.Bitcoin.DataDirectory = v
	}
	if v := os.Getenv("REDLINE_API_LISTEN_ADDRESS"); v != "" {
		c.API.ListenAddress = v
	}
	if v := os.Getenv("REDLINE_DATABASE_PATH"); v != "" {
		c.DatabasePath = v
	}
	if v := os.Getenv("REDLINE_RPC_TIMEOUT_MS"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil {
			return c, e
		}
		c.RPCTimeoutMS = n
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if c.RPCTimeoutMS < 1 {
		return fmt.Errorf("rpc_timeout_ms must be positive")
	}
	if c.Sync.MaxTipAgeSeconds < 1 || c.Sync.MinVerificationProgress <= 0 || c.Sync.MinVerificationProgress > 1 || c.Sync.MaxHeaderLag < 0 {
		return fmt.Errorf("invalid synchronization thresholds")
	}
	if c.Filesystem.FailureAvailablePercent < 0 || c.Filesystem.WarningAvailablePercent > 100 || c.Filesystem.FailureAvailablePercent > c.Filesystem.WarningAvailablePercent {
		return fmt.Errorf("invalid filesystem thresholds")
	}
	return nil
}
func (c Config) Timeout() time.Duration { return time.Duration(c.RPCTimeoutMS) * time.Millisecond }
