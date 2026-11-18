package app

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"
)

var componentIDPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{1,63}$`)

type Config struct {
	BitcoinNodes       []BitcoinNodeConfig     `json:"bitcoin_nodes"`
	ElectrumBackends   []ElectrumBackendConfig `json:"electrum_backends"`
	ElectrumThresholds ElectrumThresholds      `json:"electrum_thresholds"`
	API                APIConfig               `json:"api"`
	DatabasePath       string                  `json:"database_path"`
	RPCTimeoutMS       int                     `json:"rpc_timeout_ms"`
	Sync               SyncThresholds          `json:"synchronization"`
	Filesystem         FilesystemThresholds    `json:"filesystem"`
}

type BitcoinNodeConfig struct {
	ID            string `json:"id"`
	Enabled       bool   `json:"enabled"`
	DisplayAlias  string `json:"display_alias"`
	RPCEndpoint   string `json:"rpc_endpoint"`
	CookieFile    string `json:"cookie_file"`
	CookieEnv     string `json:"cookie_env"`
	DataDirectory string `json:"data_directory"`
}

type ElectrumBackendConfig struct {
	ID                 string `json:"id"`
	Enabled            bool   `json:"enabled"`
	DisplayAlias       string `json:"display_alias"`
	Endpoint           string `json:"endpoint"`
	TimeoutMS          int    `json:"timeout_ms"`
	BitcoinComponentID string `json:"bitcoin_component_id"`
}

// ElectrumThresholds are shared diagnostic thresholds for all electrum backends.
type ElectrumThresholds struct {
	TipWarningAgeSeconds      int64 `json:"tip_warning_age_seconds"`
	TipFailureAgeSeconds      int64 `json:"tip_failure_age_seconds"`
	LagWarningBlocks          int64 `json:"lag_warning_blocks"`
	LagFailureBlocks          int64 `json:"lag_failure_blocks"`
	MaxObservationSkewSeconds int64 `json:"max_observation_skew_seconds"`
	MaxObservationAgeSeconds  int64 `json:"max_observation_age_seconds"`
	MaxFutureSkewSeconds      int64 `json:"max_future_skew_seconds"`
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
	return Config{
		BitcoinNodes:     nil,
		ElectrumBackends: nil,
		ElectrumThresholds: ElectrumThresholds{
			TipWarningAgeSeconds: 3600, TipFailureAgeSeconds: 7200,
			LagWarningBlocks: 2, LagFailureBlocks: 10,
			MaxObservationSkewSeconds: 30, MaxObservationAgeSeconds: 60, MaxFutureSkewSeconds: 300,
		},
		API: APIConfig{ListenAddress: "127.0.0.1:8383"}, DatabasePath: "redline.db", RPCTimeoutMS: 5000,
		Sync:       SyncThresholds{MaxTipAgeSeconds: 7200, MinVerificationProgress: 0.999, MaxHeaderLag: 2},
		Filesystem: FilesystemThresholds{WarningAvailablePercent: 15, FailureAvailablePercent: 5},
	}
}

func Load(path string) (Config, error) {
	c := DefaultConfig()
	if path != "" {
		b, e := os.ReadFile(path)
		if e != nil {
			return c, e
		}
		// Reject legacy singular objects explicitly.
		var raw map[string]json.RawMessage
		if e = json.Unmarshal(b, &raw); e != nil {
			return c, fmt.Errorf("invalid configuration: %w", e)
		}
		if _, ok := raw["bitcoin"]; ok {
			return c, fmt.Errorf("invalid configuration: singular \"bitcoin\" is not supported; use bitcoin_nodes")
		}
		if _, ok := raw["electrum"]; ok {
			return c, fmt.Errorf("invalid configuration: singular \"electrum\" is not supported; use electrum_backends")
		}
		if e = json.Unmarshal(b, &c); e != nil {
			return c, fmt.Errorf("invalid configuration: %w", e)
		}
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
	if c.ElectrumThresholds.TipWarningAgeSeconds < 1 || c.ElectrumThresholds.TipFailureAgeSeconds < c.ElectrumThresholds.TipWarningAgeSeconds ||
		c.ElectrumThresholds.LagWarningBlocks < 1 || c.ElectrumThresholds.LagFailureBlocks < c.ElectrumThresholds.LagWarningBlocks ||
		c.ElectrumThresholds.MaxObservationSkewSeconds < 0 || c.ElectrumThresholds.MaxObservationAgeSeconds < 1 || c.ElectrumThresholds.MaxFutureSkewSeconds < 0 {
		return fmt.Errorf("invalid Electrum thresholds")
	}

	ids := map[string]struct{}{}
	enabledBitcoin := map[string]struct{}{}
	enabledCount := 0
	for i, node := range c.BitcoinNodes {
		if err := validateComponentID(node.ID); err != nil {
			return fmt.Errorf("bitcoin_nodes[%d]: %w", i, err)
		}
		if _, dup := ids[node.ID]; dup {
			return fmt.Errorf("duplicate component id %q", node.ID)
		}
		ids[node.ID] = struct{}{}
		if node.DisplayAlias == "" {
			return fmt.Errorf("bitcoin_nodes[%d]: display_alias is required", i)
		}
		if !node.Enabled {
			continue
		}
		enabledCount++
		enabledBitcoin[node.ID] = struct{}{}
		if node.RPCEndpoint == "" {
			return fmt.Errorf("bitcoin_nodes[%d]: rpc_endpoint is required when enabled", i)
		}
		if node.DataDirectory == "" {
			return fmt.Errorf("bitcoin_nodes[%d]: data_directory is required when enabled", i)
		}
	}
	if enabledCount < 1 {
		return fmt.Errorf("at least one enabled bitcoin node is required")
	}

	for i, backend := range c.ElectrumBackends {
		if err := validateComponentID(backend.ID); err != nil {
			return fmt.Errorf("electrum_backends[%d]: %w", i, err)
		}
		if _, dup := ids[backend.ID]; dup {
			return fmt.Errorf("duplicate component id %q", backend.ID)
		}
		ids[backend.ID] = struct{}{}
		if backend.DisplayAlias == "" {
			return fmt.Errorf("electrum_backends[%d]: display_alias is required", i)
		}
		if !backend.Enabled {
			continue
		}
		if backend.Endpoint == "" || backend.TimeoutMS < 1 {
			return fmt.Errorf("electrum_backends[%d]: enabled backend requires endpoint and timeout", i)
		}
		if _, ok := enabledBitcoin[backend.BitcoinComponentID]; !ok {
			return fmt.Errorf("electrum_backends[%d]: bitcoin_component_id %q must reference an enabled bitcoin node", i, backend.BitcoinComponentID)
		}
		endpoint, err := url.Parse(backend.Endpoint)
		if err != nil || (endpoint.Scheme != "tcp" && endpoint.Scheme != "ssl") || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Path != "" {
			return fmt.Errorf("electrum_backends[%d]: invalid Electrum endpoint", i)
		}
	}
	return nil
}

func validateComponentID(id string) error {
	if !componentIDPattern.MatchString(id) {
		return fmt.Errorf("invalid component id %q", id)
	}
	if id == "system.local" || id == "scan.aggregate" {
		return fmt.Errorf("component id %q is reserved", id)
	}
	return nil
}

func (c Config) Timeout() time.Duration {
	return time.Duration(c.RPCTimeoutMS) * time.Millisecond
}

// EnabledBitcoinNodes returns configured bitcoin nodes with enabled=true.
func (c Config) EnabledBitcoinNodes() []BitcoinNodeConfig {
	var out []BitcoinNodeConfig
	for _, n := range c.BitcoinNodes {
		if n.Enabled {
			out = append(out, n)
		}
	}
	return out
}

// EnabledElectrumBackends returns configured electrum backends with enabled=true.
func (c Config) EnabledElectrumBackends() []ElectrumBackendConfig {
	var out []ElectrumBackendConfig
	for _, b := range c.ElectrumBackends {
		if b.Enabled {
			out = append(out, b)
		}
	}
	return out
}
