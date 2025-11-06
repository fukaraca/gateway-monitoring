package model

import (
	"time"
)

// SystemStats represents the overall system statistics
type SystemStats struct {
	Timestamp    time.Time          `json:"timestamp"`
	Proc         *ProcStats         `json:"proc,omitempty"`
	Connectivity *ConnectivityStats `json:"connectivity,omitempty"`
	Netlink      *NetlinkStats      `json:"netlink,omitempty"`
}

// MemoryStats represents memory information from /proc/meminfo
type MemoryStats struct {
	MemTotal     uint64 `json:"mem_total"`
	MemFree      uint64 `json:"mem_free"`
	MemAvailable uint64 `json:"mem_available"`
	Buffers      uint64 `json:"buffers"`
	Cached       uint64 `json:"cached"`
	SwapTotal    uint64 `json:"swap_total"`
	SwapFree     uint64 `json:"swap_free"`
}

// LoadStats represents system load from /proc/loadavg
type LoadStats struct {
	Load1  float64 `json:"load_1"`
	Load5  float64 `json:"load_5"`
	Load15 float64 `json:"load_15"`
}

// NetworkStats represents network interface statistics
type NetworkStats struct {
	Interface string `json:"interface"`
	RxBytes   uint64 `json:"rx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	RxErrors  uint64 `json:"rx_errors"`
	RxDropped uint64 `json:"rx_dropped"`
	TxBytes   uint64 `json:"tx_bytes"`
	TxPackets uint64 `json:"tx_packets"`
	TxErrors  uint64 `json:"tx_errors"`
	TxDropped uint64 `json:"tx_dropped"`
}

// InterfaceInfo represents network interface information
type InterfaceInfo struct {
	Name         string   `json:"name"`
	Index        int      `json:"index"`
	MTU          int      `json:"mtu"`
	HardwareAddr string   `json:"hardware_addr"`
	Flags        []string `json:"flags"`
	State        string   `json:"state"`
	Addresses    []string `json:"addresses"`
}

// RouteInfo represents routing table information
type RouteInfo struct {
	Destination    string `json:"destination"`
	Gateway        string `json:"gateway"`
	InterfaceIndex int    `json:"interface_index"`
	Metric         int    `json:"metric"`
}

type AddressInfo struct {
	LinkAddress string `json:"link_address"`
	LinkIndex   int    `json:"link_index"`
	Flags       int    `json:"flags"`
}

type ProcStats struct {
	Memory  *MemoryStats             `json:"memory,omitempty"`
	Load    *LoadStats               `json:"load,omitempty"`
	Network map[string]*NetworkStats `json:"network,omitempty"`
}

// ConnectivityStats represents connectivity status
type ConnectivityStats struct {
	Overall     string         `json:"overall"`
	LastChecked time.Time      `json:"last_checked"`
	Ping        *PingResult    `json:"ping,omitempty"`
	DNS         *DNSResult     `json:"dns,omitempty"`
	Gateway     *GatewayResult `json:"gateway,omitempty"`
	Score       int            `json:"score"`
}

// PingResult represents ping test results
type PingResult struct {
	Target     string        `json:"target"`
	Success    bool          `json:"success"`
	RTT        time.Duration `json:"rtt"`
	PacketLoss float64       `json:"packet_loss"`
}

// DNSResult represents DNS resolution test results
type DNSResult struct {
	Target    string        `json:"target"`
	Success   bool          `json:"success"`
	Duration  time.Duration `json:"duration"`
	Addresses []string      `json:"addresses,omitempty"`
}

// GatewayResult represents gateway reachability test results
type GatewayResult struct {
	Gateway string        `json:"gateway"`
	Success bool          `json:"success"`
	RTT     time.Duration `json:"rtt"`
}

type NetlinkStats struct {
	Interfaces  map[int]*InterfaceInfo `json:"interfaces,omitempty"`
	Routes      []RouteInfo            `json:"routes,omitempty"`
	Addresses   []AddressInfo          `json:"addresses,omitempty"`
	LastChecked time.Time              `json:"last_checked"`
}
