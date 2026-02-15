package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/fukaraca/gateway-monitoring/internal/config"
	"github.com/fukaraca/gateway-monitoring/internal/connectivity"
	"github.com/fukaraca/gateway-monitoring/internal/metric"
	"github.com/fukaraca/gateway-monitoring/internal/model"
	"github.com/fukaraca/gateway-monitoring/internal/netlink"
	"github.com/fukaraca/gateway-monitoring/internal/proc"
	"go.uber.org/zap"
)

type GatewayMonitor struct {
	logger     *zap.Logger
	components map[string]Monitor
	health     *metric.SystemHealth
	exporter   *metric.Exporter

	wg             sync.WaitGroup
	mu             sync.RWMutex
	stats          model.SystemStats
	updateInterval time.Duration
	version        string

	ctx    context.Context
	cancel context.CancelFunc
}

func NewGatewayMonitor(cfg *config.Config) *GatewayMonitor {
	comps := make(map[string]Monitor)
	compsHealth := make([]metric.ComponentHealth, 0)

	if cfg.Components.ConnectivityMonitor {
		mon := connectivity.NewMonitor(cfg.Connectivity, cfg.Logger)
		comps[mon.Name()] = mon
		compsHealth = append(compsHealth, metric.ComponentHealth{
			Name:        mon.Name(),
			Status:      metric.HealthStatusUnknown,
			LastChecked: time.Time{},
		})
	}

	if cfg.Components.ProcMonitor {
		mon := proc.NewMonitor(cfg.Proc, cfg.Logger)
		comps[mon.Name()] = mon
		compsHealth = append(compsHealth, metric.ComponentHealth{
			Name:        mon.Name(),
			Status:      metric.HealthStatusUnknown,
			LastChecked: time.Time{},
		})
	}

	if cfg.Components.NetlinkMonitor {
		mon := netlink.NewMonitor(cfg.Netlink, cfg.Logger)
		comps[mon.Name()] = mon
		compsHealth = append(compsHealth, metric.ComponentHealth{
			Name:        mon.Name(),
			Status:      metric.HealthStatusUnknown,
			LastChecked: time.Time{},
		})
	}
	health := &metric.SystemHealth{
		Overall:    metric.HealthStatusUnknown,
		Timestamp:  time.Now(),
		Components: compsHealth,
		Uptime:     0,
	}

	return &GatewayMonitor{
		logger:         cfg.Logger,
		components:     comps,
		updateInterval: cfg.MonitoringInterval,
		health:         health,
		exporter:       metric.NewExporter(cfg.PrometheusServer, cfg.Logger),
		version:        cfg.Version,
	}
}

func (gm *GatewayMonitor) Start(ctx context.Context) error {
	gm.ctx, gm.cancel = context.WithCancel(ctx)

	gm.logger.Info("Starting Gateway Monitor...")

	// Start all monitoring components
	for s, mon := range gm.components {
		if err := mon.Start(gm.ctx); err != nil {
			gm.Stop() // stop/close/wait all resources
			return fmt.Errorf("failed to start %s monitor: %w", s, err)
		}
	}

	// Start the main monitoring loop
	gm.wg.Go(gm.monitoringLoop) // TODO errors ignored. Use errgroup if errors matter.
	gm.wg.Go(func() { gm.exporter.Start(ctx) })

	return nil
}

func (gm *GatewayMonitor) Stop() error {
	gm.logger.Info("Stopping Gateway Monitor...")
	gm.cancel()

	var err error
	for s, mon := range gm.components {
		if errIn := mon.Stop(); errIn != nil {
			gm.logger.Error("monitor couldn't be stopped", zap.Error(err), zap.String("component", s))
			err = errors.Join(err, errIn)
		}
	}

	gm.exporter.Stop()
	gm.wg.Wait()
	gm.logger.Info("Gateway Monitor stopped", zap.Bool("isFailed", err != nil))
	return err
}

func (gm *GatewayMonitor) monitoringLoop() {
	ticker := time.NewTicker(gm.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-gm.ctx.Done():
			return
		case <-ticker.C:
			gm.updateStats()
			gm.printStats()
		}
	}
}

func (gm *GatewayMonitor) updateStats() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.stats.Timestamp = time.Now()
	healthScore := 100.0
	for s, monitor := range gm.components {
		if err := monitor.UpdateStats(&gm.stats); err != nil {
			healthScore -= 30 // TODO each monitor has their own score instead of constant 30, also drop 30, we don't like magic numers
			gm.logger.Error("component monitor failed", zap.Error(err), zap.String("component", s))
		}
	}

	gm.health.Uptime = time.Since(gm.health.Timestamp) // TODO a decent health assessment
	gm.health.Overall = metric.HealthStatusHealthy

	metric.SystemHealthStatus.Set(healthScore / 10)
	metric.SystemHealthLastChecked.SetToCurrentTime()
	metric.SystemUptime.Set(gm.health.Uptime.Seconds())
}

func (gm *GatewayMonitor) printStats() {
	data, err := json.MarshalIndent(gm.GetStats(), "", "  ") // TODO using a buffer with json.Encoder can be another option
	if err != nil {
		gm.logger.Error("Error marshaling stats", zap.Error(err))
		return
	}
	if gm.version != "testing" {
		fmt.Println(string(data))
		fmt.Println("---")
	}
}

func (gm *GatewayMonitor) GetStats() model.SystemStats {
	gm.mu.RLock()
	s := gm.stats
	gm.mu.RUnlock()
	return s
}

func (gm *GatewayMonitor) GetMonitor(name string) (Monitor, error) {
	if mon, ok := gm.components[name]; ok {
		return mon, nil
	}
	return nil, errors.New("monitor not found")
}
