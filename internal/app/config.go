package app

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Bitcoin      BitcoinConfig        `json:"bitcoin"`
	Electrum     ElectrumConfig       `json:"electrum"`
	API          APIConfig            `json:"api"`
	DatabasePath string               `json:"database_path"`
	RPCTimeoutMS int                  `json:"rpc_timeout_ms"`
	Sync         SyncThresholds       `json:"synchronization"`
	Filesystem   FilesystemThresholds `json:"filesystem"`
}
type ElectrumConfig struct {
	Enabled                   bool   `json:"enabled"`
	Endpoint                  string `json:"endpoint"`
	TimeoutMS                 int    `json:"timeout_ms"`
	BitcoinComponentID        string `json:"bitcoin_component_id"`
	TipWarningAgeSeconds      int64  `json:"tip_warning_age_seconds"`
	TipFailureAgeSeconds      int64  `json:"tip_failure_age_seconds"`
	LagWarningBlocks          int64  `json:"lag_warning_blocks"`
	LagFailureBlocks          int64  `json:"lag_failure_blocks"`
	MaxObservationSkewSeconds int64  `json:"max_observation_skew_seconds"`
	MaxObservationAgeSeconds  int64  `json:"max_observation_age_seconds"`
	MaxFutureSkewSeconds      int64  `json:"max_future_skew_seconds"`
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
	return Config{Electrum: ElectrumConfig{TimeoutMS: 5000, TipWarningAgeSeconds: 3600, TipFailureAgeSeconds: 7200, LagWarningBlocks: 2, LagFailureBlocks: 10, MaxObservationSkewSeconds: 30, MaxObservationAgeSeconds: 60, MaxFutureSkewSeconds: 300}, API: APIConfig{"127.0.0.1:8383"}, DatabasePath: "redline.db", RPCTimeoutMS: 5000, Sync: SyncThresholds{7200, .999, 2}, Filesystem: FilesystemThresholds{15, 5}}
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
	if v := os.Getenv("REDLINE_ELECTRUM_ENABLED"); v != "" {
		enabled, e := strconv.ParseBool(v)
		if e != nil {
			return c, fmt.Errorf("invalid Electrum enabled override")
		}
		c.Electrum.Enabled = enabled
	}
	if v := os.Getenv("REDLINE_ELECTRUM_ENDPOINT"); v != "" {
		c.Electrum.Endpoint = v
	}
	if v := os.Getenv("REDLINE_ELECTRUM_TIMEOUT_MS"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil {
			return c, fmt.Errorf("invalid Electrum timeout override")
		}
		c.Electrum.TimeoutMS = n
	}
	if v := os.Getenv("REDLINE_ELECTRUM_BITCOIN_COMPONENT_ID"); v != "" {
		c.Electrum.BitcoinComponentID = v
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
	if c.Electrum.Enabled {
		if c.Electrum.Endpoint == "" || c.Electrum.BitcoinComponentID != "bitcoin.primary" || c.Electrum.TimeoutMS < 1 {
			return fmt.Errorf("enabled Electrum requires endpoint, timeout, and bitcoin.primary binding")
		}
		endpoint, err := url.Parse(c.Electrum.Endpoint)
		if err != nil || (endpoint.Scheme != "tcp" && endpoint.Scheme != "ssl") || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Path != "" {
			return fmt.Errorf("invalid Electrum endpoint")
		}
	}
	if c.Electrum.TipWarningAgeSeconds < 1 || c.Electrum.TipFailureAgeSeconds < c.Electrum.TipWarningAgeSeconds ||
		c.Electrum.LagWarningBlocks < 1 || c.Electrum.LagFailureBlocks < c.Electrum.LagWarningBlocks ||
		c.Electrum.MaxObservationSkewSeconds < 0 || c.Electrum.MaxObservationAgeSeconds < 1 || c.Electrum.MaxFutureSkewSeconds < 0 {
		return fmt.Errorf("invalid Electrum thresholds")
	}
	return nil
}
func (c Config) Timeout() time.Duration { return time.Duration(c.RPCTimeoutMS) * time.Millisecond }
