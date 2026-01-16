package config

import (
	"fmt"
	"time"
)

// Config represents the complete configuration for the load testing tool
type Config struct {
	ShardDistributor ShardDistributorConfig `yaml:"shardDistributor"`
	Bench            BenchConfig            `yaml:"bench"`
	Logging          LoggingConfig          `yaml:"logging"`
}

// Validate checks the Config for validity.
func (c *Config) Validate() error {
	if err := c.ShardDistributor.Validate(); err != nil {
		return err
	}
	if err := c.Bench.Validate(); err != nil {
		return err
	}
	if err := c.Logging.Validate(); err != nil {
		return err
	}
	return nil
}

// ShardDistributorConfig contains ShardDistributor connection settings
type ShardDistributorConfig struct {
	ServiceName string        `yaml:"serviceName"`
	Address     string        `yaml:"address"`
	Timeout     time.Duration `yaml:"timeout"`
}

// Validate checks the ShardDistributorConfig for validity.
func (s *ShardDistributorConfig) Validate() error {
	if s.ServiceName == "" {
		return fmt.Errorf("shardDistributor.serviceName is required")
	}
	if s.Address == "" {
		return fmt.Errorf("shardDistributor.address is required")
	}
	if s.Timeout <= 0 {
		return fmt.Errorf("shardDistributor.timeout must be positive")
	}
	return nil
}

// BenchConfig contains load testing configuration
type BenchConfig struct {
	Namespaces []NamespaceConfig `yaml:"namespaces"`
}

// Validate checks the BenchConfig for validity.
func (b *BenchConfig) Validate() error {
	if len(b.Namespaces) == 0 {
		return fmt.Errorf("bench.namespaces must not be empty")
	}
	for i, ns := range b.Namespaces {
		if err := ns.Validate(); err != nil {
			return fmt.Errorf("bench.namespaces[%d]: %w", i, err)
		}
	}
	return nil
}

// NamespaceConfig contains per-namespace load testing configuration
type NamespaceConfig struct {
	// Name of the namespace
	Name string `yaml:"name"`

	// Type of namespace: "fixed" or "ephemeral"
	Type string `yaml:"type"`

	// NumSpectators is the number of spectator instances running by the bench for this namespace
	NumSpectators uint `yaml:"numSpectators"`

	// NumExecutors is the number of executor instances running by the bench for this namespace
	NumExecutors uint `yaml:"numExecutors"`

	// ExecutorRecreationPercentage is the percentage of executors that will be recreated within ExecutorReassignmentInterval
	// Default: 0
	// Values: [0.0, 100.0]
	// Meaning: 0 means no executor recreation, 100.0 means all executors will be recreated within the interval
	ExecutorRecreationPercentage float64 `yaml:"executorReassignmentPercentage"`

	// ExecutorReassignmentInterval is the interval at which executors are recreated
	// Default: 1 minute
	ExecutorRecreationInterval time.Duration `yaml:"executorReassignmentInterval,omitempty"`

	// ShardLoadAlgorithm is the algorithm used to calculate shard load
	// Default: "constant"
	// Values: "constant", "linear", "random"
	// Meaning:
	//   - "constant": all shards have the same load of 1.0
	//   - "shard-id": shard load based on shard ID (e.g., shard-1 has load 1.0, shard-2 has load 2.0, etc.)
	//   - "random": shard load is a random value between 0.1 and 100.0
	ShardLoadAlgorithm string `yaml:"shardLoadAlgorithm"`

	// EphemeralShardCreationRPS is the rate at which random ephemeral shards are created in this namespace
	// by this bench instance
	// Default: 0 (no ephemeral shards created)
	// Only applicable for ephemeral namespaces
	EphemeralShardCreationRPS float64 `yaml:"ephemeralShardCreationRPS,omitempty"`

	// EphemeralShardLifetime is the lifetime of ephemeral shards created in this namespace. After this duration,
	// the shards will be considered expired and will be reported as Done to the shard distributor.
	// Default: 10 minutes
	// Only applicable for ephemeral namespaces
	EphemeralShardLifetime time.Duration `yaml:"ephemeralShardLifetime,omitempty"`
	// Validate checks the NamespaceConfig for validity.
}

func (n *NamespaceConfig) Validate() error {
	if n.Name == "" {
		return fmt.Errorf("namespace.name is required")
	}
	if n.Type != "fixed" && n.Type != "ephemeral" {
		return fmt.Errorf("namespace.type must be 'fixed' or 'ephemeral'")
	}
	if n.NumSpectators == 0 {
		return fmt.Errorf("namespace.numSpectators must be positive")
	}
	if n.NumExecutors == 0 {
		return fmt.Errorf("namespace.numExecutors must be positive")
	}
	if n.ExecutorRecreationPercentage < 0.0 || n.ExecutorRecreationPercentage > 100.0 {
		return fmt.Errorf("namespace.executorReassignmentPercentage must be in [0.0, 100.0]")
	}
	if n.Type == "ephemeral" {
		if n.EphemeralShardCreationRPS < 0.0 {
			return fmt.Errorf("namespace.ephemeralShardCreationRPS must be non-negative")
		}
		if n.EphemeralShardLifetime < 0 {
			return fmt.Errorf("namespace.ephemeralShardLifetime must be non-negative")
		}
	}
	return nil
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	// Level is the logging level: "debug", "info", "warn", "error"
	Level string `yaml:"level"`
}

// Validate checks the LoggingConfig for validity.
func (l *LoggingConfig) Validate() error {
	switch l.Level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("logging.level must be one of: debug, info, warn, error")
	}
}
