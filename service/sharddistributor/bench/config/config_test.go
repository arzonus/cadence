package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				ShardDistributor: ShardDistributorConfig{
					ServiceName: "shard-distributor",
					Address:     "localhost:7600",
					Timeout:     5 * time.Second,
				},
				Bench: BenchConfig{
					Namespaces: []NamespaceConfig{
						{
							Name:                         "test-namespace",
							Type:                         "fixed",
							NumSpectators:                10,
							NumExecutors:                 5,
							ExecutorRecreationPercentage: 10.0,
						},
					},
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			wantErr: false,
		},
		{
			name: "missing service name",
			config: &Config{
				ShardDistributor: ShardDistributorConfig{
					ServiceName: "",
					Address:     "localhost:7600",
					Timeout:     5 * time.Second,
				},
				Bench: BenchConfig{
					Namespaces: []NamespaceConfig{
						{
							Name:          "test-namespace",
							Type:          "fixed",
							NumSpectators: 10,
							NumExecutors:  5,
						},
					},
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			wantErr: true,
			errMsg:  "shardDistributor.serviceName is required",
		},
		{
			name: "missing address",
			config: &Config{
				ShardDistributor: ShardDistributorConfig{
					ServiceName: "shard-distributor",
					Address:     "",
					Timeout:     5 * time.Second,
				},
				Bench: BenchConfig{
					Namespaces: []NamespaceConfig{
						{
							Name:          "test-namespace",
							Type:          "fixed",
							NumSpectators: 10,
							NumExecutors:  5,
						},
					},
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			wantErr: true,
			errMsg:  "shardDistributor.address is required",
		},
		{
			name: "invalid timeout",
			config: &Config{
				ShardDistributor: ShardDistributorConfig{
					ServiceName: "shard-distributor",
					Address:     "localhost:7600",
					Timeout:     0,
				},
				Bench: BenchConfig{
					Namespaces: []NamespaceConfig{
						{
							Name:          "test-namespace",
							Type:          "fixed",
							NumSpectators: 10,
							NumExecutors:  5,
						},
					},
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			wantErr: true,
			errMsg:  "shardDistributor.timeout must be positive",
		},
		{
			name: "empty namespaces",
			config: &Config{
				ShardDistributor: ShardDistributorConfig{
					ServiceName: "shard-distributor",
					Address:     "localhost:7600",
					Timeout:     5 * time.Second,
				},
				Bench: BenchConfig{
					Namespaces: []NamespaceConfig{},
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			wantErr: true,
			errMsg:  "bench.namespaces must not be empty",
		},
		{
			name: "invalid logging level",
			config: &Config{
				ShardDistributor: ShardDistributorConfig{
					ServiceName: "shard-distributor",
					Address:     "localhost:7600",
					Timeout:     5 * time.Second,
				},
				Bench: BenchConfig{
					Namespaces: []NamespaceConfig{
						{
							Name:          "test-namespace",
							Type:          "fixed",
							NumSpectators: 10,
							NumExecutors:  5,
						},
					},
				},
				Logging: LoggingConfig{
					Level: "invalid",
				},
			},
			wantErr: true,
			errMsg:  "logging.level must be one of: debug, info, warn, error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestShardDistributorConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ShardDistributorConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: ShardDistributorConfig{
				ServiceName: "shard-distributor",
				Address:     "localhost:7600",
				Timeout:     5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "missing service name",
			config: ShardDistributorConfig{
				ServiceName: "",
				Address:     "localhost:7600",
				Timeout:     5 * time.Second,
			},
			wantErr: true,
			errMsg:  "shardDistributor.serviceName is required",
		},
		{
			name: "missing address",
			config: ShardDistributorConfig{
				ServiceName: "shard-distributor",
				Address:     "",
				Timeout:     5 * time.Second,
			},
			wantErr: true,
			errMsg:  "shardDistributor.address is required",
		},
		{
			name: "zero timeout",
			config: ShardDistributorConfig{
				ServiceName: "shard-distributor",
				Address:     "localhost:7600",
				Timeout:     0,
			},
			wantErr: true,
			errMsg:  "shardDistributor.timeout must be positive",
		},
		{
			name: "negative timeout",
			config: ShardDistributorConfig{
				ServiceName: "shard-distributor",
				Address:     "localhost:7600",
				Timeout:     -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "shardDistributor.timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBenchConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  BenchConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with one namespace",
			config: BenchConfig{
				Namespaces: []NamespaceConfig{
					{
						Name:          "test-namespace",
						Type:          "fixed",
						NumSpectators: 10,
						NumExecutors:  5,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with multiple namespaces",
			config: BenchConfig{
				Namespaces: []NamespaceConfig{
					{
						Name:          "fixed-namespace",
						Type:          "fixed",
						NumSpectators: 10,
						NumExecutors:  5,
					},
					{
						Name:                      "ephemeral-namespace",
						Type:                      "ephemeral",
						NumSpectators:             20,
						NumExecutors:              10,
						EphemeralShardCreationRPS: 5.0,
						EphemeralShardLifetime:    10 * time.Minute,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty namespaces",
			config: BenchConfig{
				Namespaces: []NamespaceConfig{},
			},
			wantErr: true,
			errMsg:  "bench.namespaces must not be empty",
		},
		{
			name: "nil namespaces",
			config: BenchConfig{
				Namespaces: nil,
			},
			wantErr: true,
			errMsg:  "bench.namespaces must not be empty",
		},
		{
			name: "invalid namespace config",
			config: BenchConfig{
				Namespaces: []NamespaceConfig{
					{
						Name:          "",
						Type:          "fixed",
						NumSpectators: 10,
						NumExecutors:  5,
					},
				},
			},
			wantErr: true,
			errMsg:  "bench.namespaces[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNamespaceConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  NamespaceConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid fixed namespace",
			config: NamespaceConfig{
				Name:          "test-fixed",
				Type:          "fixed",
				NumSpectators: 10,
				NumExecutors:  5,
			},
			wantErr: false,
		},
		{
			name: "valid ephemeral namespace",
			config: NamespaceConfig{
				Name:                      "test-ephemeral",
				Type:                      "ephemeral",
				NumSpectators:             20,
				NumExecutors:              10,
				EphemeralShardCreationRPS: 5.0,
				EphemeralShardLifetime:    10 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "valid with executor recreation",
			config: NamespaceConfig{
				Name:                         "test-namespace",
				Type:                         "fixed",
				NumSpectators:                10,
				NumExecutors:                 5,
				ExecutorRecreationPercentage: 50.0,
				ExecutorRecreationInterval:   2 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: NamespaceConfig{
				Name:          "",
				Type:          "fixed",
				NumSpectators: 10,
				NumExecutors:  5,
			},
			wantErr: true,
			errMsg:  "namespace.name is required",
		},
		{
			name: "invalid type",
			config: NamespaceConfig{
				Name:          "test-namespace",
				Type:          "invalid",
				NumSpectators: 10,
				NumExecutors:  5,
			},
			wantErr: true,
			errMsg:  "namespace.type must be 'fixed' or 'ephemeral'",
		},
		{
			name: "zero spectators",
			config: NamespaceConfig{
				Name:          "test-namespace",
				Type:          "fixed",
				NumSpectators: 0,
				NumExecutors:  5,
			},
			wantErr: true,
			errMsg:  "namespace.numSpectators must be positive",
		},
		{
			name: "zero executors",
			config: NamespaceConfig{
				Name:          "test-namespace",
				Type:          "fixed",
				NumSpectators: 10,
				NumExecutors:  0,
			},
			wantErr: true,
			errMsg:  "namespace.numExecutors must be positive",
		},
		{
			name: "negative executor recreation percentage",
			config: NamespaceConfig{
				Name:                         "test-namespace",
				Type:                         "fixed",
				NumSpectators:                10,
				NumExecutors:                 5,
				ExecutorRecreationPercentage: -1.0,
			},
			wantErr: true,
			errMsg:  "namespace.executorReassignmentPercentage must be in [0.0, 100.0]",
		},
		{
			name: "executor recreation percentage > 100",
			config: NamespaceConfig{
				Name:                         "test-namespace",
				Type:                         "fixed",
				NumSpectators:                10,
				NumExecutors:                 5,
				ExecutorRecreationPercentage: 101.0,
			},
			wantErr: true,
			errMsg:  "namespace.executorReassignmentPercentage must be in [0.0, 100.0]",
		},
		{
			name: "negative ephemeral shard creation RPS",
			config: NamespaceConfig{
				Name:                      "test-ephemeral",
				Type:                      "ephemeral",
				NumSpectators:             10,
				NumExecutors:              5,
				EphemeralShardCreationRPS: -1.0,
			},
			wantErr: true,
			errMsg:  "namespace.ephemeralShardCreationRPS must be non-negative",
		},
		{
			name: "negative ephemeral shard lifetime",
			config: NamespaceConfig{
				Name:                   "test-ephemeral",
				Type:                   "ephemeral",
				NumSpectators:          10,
				NumExecutors:           5,
				EphemeralShardLifetime: -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "namespace.ephemeralShardLifetime must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoggingConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  LoggingConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "debug level",
			config:  LoggingConfig{Level: "debug"},
			wantErr: false,
		},
		{
			name:    "info level",
			config:  LoggingConfig{Level: "info"},
			wantErr: false,
		},
		{
			name:    "warn level",
			config:  LoggingConfig{Level: "warn"},
			wantErr: false,
		},
		{
			name:    "error level",
			config:  LoggingConfig{Level: "error"},
			wantErr: false,
		},
		{
			name:    "invalid level",
			config:  LoggingConfig{Level: "invalid"},
			wantErr: true,
			errMsg:  "logging.level must be one of: debug, info, warn, error",
		},
		{
			name:    "empty level",
			config:  LoggingConfig{Level: ""},
			wantErr: true,
			errMsg:  "logging.level must be one of: debug, info, warn, error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNamespaceConfig_Defaults(t *testing.T) {
	tests := []struct {
		name     string
		config   NamespaceConfig
		expected NamespaceConfig
	}{
		{
			name: "ephemeral namespace with zero values uses defaults",
			config: NamespaceConfig{
				Name:                      "test-ephemeral",
				Type:                      "ephemeral",
				NumSpectators:             10,
				NumExecutors:              5,
				EphemeralShardCreationRPS: 0,
				EphemeralShardLifetime:    0,
			},
			expected: NamespaceConfig{
				Name:                      "test-ephemeral",
				Type:                      "ephemeral",
				NumSpectators:             10,
				NumExecutors:              5,
				EphemeralShardCreationRPS: 0,
				EphemeralShardLifetime:    0,
			},
		},
		{
			name: "executor recreation interval defaults to zero",
			config: NamespaceConfig{
				Name:                         "test-namespace",
				Type:                         "fixed",
				NumSpectators:                10,
				NumExecutors:                 5,
				ExecutorRecreationPercentage: 10.0,
			},
			expected: NamespaceConfig{
				Name:                         "test-namespace",
				Type:                         "fixed",
				NumSpectators:                10,
				NumExecutors:                 5,
				ExecutorRecreationPercentage: 10.0,
				ExecutorRecreationInterval:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config)
		})
	}
}

func TestNamespaceConfig_EdgeCases(t *testing.T) {
	t.Run("max uint values", func(t *testing.T) {
		config := NamespaceConfig{
			Name:          "test-namespace",
			Type:          "fixed",
			NumSpectators: ^uint(0), // max uint
			NumExecutors:  ^uint(0), // max uint
		}
		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("100% executor recreation percentage", func(t *testing.T) {
		config := NamespaceConfig{
			Name:                         "test-namespace",
			Type:                         "fixed",
			NumSpectators:                10,
			NumExecutors:                 5,
			ExecutorRecreationPercentage: 100.0,
		}
		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("0% executor recreation percentage", func(t *testing.T) {
		config := NamespaceConfig{
			Name:                         "test-namespace",
			Type:                         "fixed",
			NumSpectators:                10,
			NumExecutors:                 5,
			ExecutorRecreationPercentage: 0.0,
		}
		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("very high ephemeral RPS", func(t *testing.T) {
		config := NamespaceConfig{
			Name:                      "test-ephemeral",
			Type:                      "ephemeral",
			NumSpectators:             10,
			NumExecutors:              5,
			EphemeralShardCreationRPS: 1000000.0,
		}
		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("very long ephemeral lifetime", func(t *testing.T) {
		config := NamespaceConfig{
			Name:                   "test-ephemeral",
			Type:                   "ephemeral",
			NumSpectators:          10,
			NumExecutors:           5,
			EphemeralShardLifetime: 24 * time.Hour * 365, // 1 year
		}
		err := config.Validate()
		require.NoError(t, err)
	})
}
