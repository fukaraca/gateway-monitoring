package config

import (
	"time"

	"go.uber.org/zap"
)

type Config struct {
	Logger             *zap.Logger
	Components         *ComponentsConfig
	Proc               *ProcConfig
	Connectivity       *ConnectivityConfig
	Netlink            *NetlinkConfig
	Logging            *LoggingConfig
	Thresholds         *ThresholdsConfig
	Interfaces         []string
	Version            string
	MonitoringInterval time.Duration           `mapstructure:"monitoring_interval"`
	PrometheusServer   *PrometheusServerConfig `mapstructure:"prom_server"`
}

type ComponentsConfig struct {
	ProcMonitor         bool `mapstructure:"proc_monitor"`
	NetlinkMonitor      bool `mapstructure:"netlink_monitor"`
	ConnectivityMonitor bool `mapstructure:"connectivity_monitor"`
}

type LoggingConfig struct {
	Level  string
	Format string
	Output string
}

type ConnectivityConfig struct {
	PingTargets   []string `mapstructure:"ping_targets"`
	DnsTargets    []string `mapstructure:"dns_targets"`
	RecordTypes   []string `mapstructure:"record_types"`
	DNSServers    []string `mapstructure:"dns_servers"`
	Gateway       string
	Timeout       time.Duration
	CheckInterval time.Duration `mapstructure:"check_interval"`
}

type ProcConfig struct {
	CheckInterval time.Duration `mapstructure:"check_interval"`
	MeminfoPath   string        `mapstructure:"meminfo_path"`
	LoadavgPath   string        `mapstructure:"loadavg_path"`
	NetdevPath    string        `mapstructure:"netdev_path"`
}

type NetlinkConfig struct {
	CheckInterval time.Duration `mapstructure:"check_interval"`
}

type ThresholdsConfig struct {
	Memory       MemoryTH
	Load         LoadTh
	Connectivity ConnectivityTH
}

type MemoryTH struct {
	WarningPercent  float64 `mapstructure:"warning_percent"`
	CriticalPercent float64 `mapstructure:"critical_percent"`
}

type LoadTh struct {
	Warning1Min  float64 `mapstructure:"warning_1min"`
	Critical1Min float64 `mapstructure:"critical_1min"`
}

type ConnectivityTH struct {
	WarningScore  float64 `mapstructure:"warning_score"`
	CriticalScore float64 `mapstructure:"critical_score"`
}

type PrometheusServerConfig struct {
	Addr      string
	Namespace string
}
