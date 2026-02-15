package proc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/fukaraca/gateway-monitoring/internal/config"
	"github.com/fukaraca/gateway-monitoring/internal/metric"
	"github.com/fukaraca/gateway-monitoring/internal/model"
	"go.uber.org/zap"
)

const Name = "proc"

// Monitor handles proc filesystem monitoring
type Monitor struct {
	cfg    *config.ProcConfig
	logger *zap.Logger

	mu          sync.RWMutex
	meminfoFile *os.File
	loadavgFile *os.File
	netdevFile  *os.File
	cancel      context.CancelFunc
	running     bool
}

// NewMonitor creates a new proc filesystem monitor
func NewMonitor(cfg *config.ProcConfig, logger *zap.Logger) *Monitor {
	return &Monitor{cfg: cfg, logger: logger}
}

func (m *Monitor) Name() string {
	return Name
}

// Start initializes the monitor. It is caller's responsibility to Stop() the monitor.
func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return fmt.Errorf("monitor already running")
	}

	// TODO: maybe open/read/close(no seek/EOF burden) per interval instead of keepit-open
	ctx, cancel := context.WithCancel(ctx) // this monitor doesn't use ctx, but we pass it as convention
	m.cancel = cancel

	var err error
	if m.meminfoFile != nil {
		m.meminfoFile.Close()
	}
	m.meminfoFile, err = os.Open(m.cfg.MeminfoPath)
	if err != nil {
		return fmt.Errorf("failed to open meminfo at %s: %w", m.cfg.MeminfoPath, err)
	}

	if m.loadavgFile != nil {
		m.loadavgFile.Close()
	}
	m.loadavgFile, err = os.Open(m.cfg.LoadavgPath)
	if err != nil {
		// meminfo fd left open but gateway-monitor will close on err
		return fmt.Errorf("failed to open loadavg at %s: %w", m.cfg.LoadavgPath, err)
	}

	if m.netdevFile != nil {
		m.netdevFile.Close()
	}
	m.netdevFile, err = os.Open(m.cfg.NetdevPath)
	if err != nil {
		return fmt.Errorf("failed to open net/dev at %s: %w", m.cfg.NetdevPath, err)
	}

	m.running = true
	m.logger.Info("Proc monitoring started...")
	return nil
}

// GetMemoryStats reads and parses /proc/meminfo
func (m *Monitor) GetMemoryStats() (*model.MemoryStats, error) {
	scanner := bufio.NewScanner(m.meminfoFile) // TODO bufio.Reader can be efficient option
	scanner.Split(bufio.ScanLines)

	stats := &model.MemoryStats{}
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		switch parts[0] {
		case "MemTotal:":
			stats.MemTotal, _ = parseMemoryValue(line) // TODO we should pass parts[1:] to parse instead of reslicing in parseMemoryValue again
		case "MemFree:":
			stats.MemFree, _ = parseMemoryValue(line)
		case "MemAvailable:":
			stats.MemAvailable, _ = parseMemoryValue(line)
		case "Buffers:":
			stats.Buffers, _ = parseMemoryValue(line)
		case "Cached:":
			stats.Cached, _ = parseMemoryValue(line)
		case "SwapTotal:":
			stats.SwapTotal, _ = parseMemoryValue(line)
		case "SwapFree:":
			stats.SwapFree, _ = parseMemoryValue(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	_, err := m.meminfoFile.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// GetLoadStats reads and parses /proc/loadavg
func (m *Monitor) GetLoadStats() (*model.LoadStats, error) {
	scanner := bufio.NewScanner(m.loadavgFile)
	scanner.Split(bufio.ScanLines)

	stats := &model.LoadStats{}
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		var err error
		stats.Load1, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return nil, err
		}

		stats.Load5, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, err
		}

		stats.Load15, err = strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return nil, err
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	_, err := m.loadavgFile.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// GetNetworkStats reads and parses /proc/net/dev
func (m *Monitor) GetNetworkStats() (map[string]*model.NetworkStats, error) {
	scanner := bufio.NewScanner(m.netdevFile)
	scanner.Split(bufio.ScanLines)

	stats := make(map[string]*model.NetworkStats)
	skipHeader := true
	for scanner.Scan() {
		line := scanner.Text()
		if skipHeader {
			skipHeader = false
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 17 {
			continue
		}

		iface := parts[0]
		iface = strings.TrimSuffix(iface, ":")

		if iface == "lo" {
			continue
		}

		networkStats := &model.NetworkStats{
			Interface: iface,
			RxBytes:   parseUint64(parts[1]),
			RxPackets: parseUint64(parts[2]),
			RxErrors:  parseUint64(parts[3]),
			RxDropped: parseUint64(parts[4]),
			TxBytes:   parseUint64(parts[9]),
			TxPackets: parseUint64(parts[10]),
			TxErrors:  parseUint64(parts[11]),
			TxDropped: parseUint64(parts[12]),
		}

		stats[iface] = networkStats
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	_, err := m.netdevFile.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (m *Monitor) GetStats() (*model.ProcStats, error) {
	out := model.ProcStats{}
	var errM, errL, errN error
	out.Memory, errM = m.GetMemoryStats() // TODO too many new map/slice allocations, we only need to check err and modify in place
	out.Load, errL = m.GetLoadStats()
	out.Network, errN = m.GetNetworkStats()
	err := errors.Join(errM, errL, errN)
	return &out, err
}

// UpdateStats sets system stats
func (m *Monitor) UpdateStats(stats *model.SystemStats) error {
	procStats, err := m.GetStats()
	var s float64
	if err == nil {
		s = 10
	}
	stats.Proc = procStats

	metric.ComponentHealthLastChecked.WithLabelValues(Name).SetToCurrentTime()
	metric.ComponentHealthStatus.WithLabelValues(Name).Set(s)
	return err
}

// Stop closes file handles (this method is missing from the interface)
func (m *Monitor) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var err error

	if m.meminfoFile != nil {
		err = errors.Join(err, m.meminfoFile.Close())
	}
	if m.loadavgFile != nil {
		err = errors.Join(err, m.loadavgFile.Close())
	}
	if m.netdevFile != nil {
		err = errors.Join(err, m.netdevFile.Close())
	}

	m.running = false
	m.logger.Info("Proc monitor stopped")
	return err
}

// parseMemoryValue parses memory values from /proc/meminfo (values are in kB)
func parseMemoryValue(line string) (uint64, error) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid memory line format")
	}

	// Remove 'kB' suffix if present
	valueStr := strings.TrimSuffix(parts[1], "kB")
	value, err := strconv.ParseUint(valueStr, 10, 64)
	if err != nil {
		return 0, err
	}

	// Convert from kB to bytes
	return value * 1024, nil
}

func parseUint64(s string) uint64 {
	value, _ := strconv.ParseUint(s, 10, 64)
	return value
}
