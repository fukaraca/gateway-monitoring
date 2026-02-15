package connectivity

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/fukaraca/gateway-monitoring/internal/config"
	"github.com/fukaraca/gateway-monitoring/internal/metric"
	"github.com/fukaraca/gateway-monitoring/internal/model"
	probing "github.com/prometheus-community/pro-bing"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

const Name = "connectivity"

// Monitor handles connectivity monitoring
type Monitor struct {
	status      *model.ConnectivityStats
	pingTargets []string
	dnsTargets  []string
	recordTypes []string
	dnsServers  []string
	gateway     string

	mu            sync.RWMutex
	wg            sync.WaitGroup
	logger        *zap.Logger
	running       bool
	checkInterval time.Duration
	timeout       time.Duration
	cancel        context.CancelFunc
}

// NewMonitor creates a new connectivity monitor
func NewMonitor(cfg *config.ConnectivityConfig, logger *zap.Logger) *Monitor {
	return &Monitor{
		status: &model.ConnectivityStats{
			Overall: "unknown",
		},
		pingTargets:   cfg.PingTargets,
		dnsTargets:    cfg.DnsTargets,
		recordTypes:   cfg.RecordTypes,
		dnsServers:    cfg.DNSServers,
		gateway:       cfg.Gateway,
		checkInterval: cfg.CheckInterval,
		timeout:       cfg.Timeout,
		logger:        logger,
	}
}

func (m *Monitor) Name() string {
	return Name
}

// Start begins connectivity monitoring
func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return fmt.Errorf("monitor already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	// Discover default gateway
	if err := m.discoverGateway(); err != nil {
		return fmt.Errorf("failed to discover gateway: %w", err)
	}

	// Start monitoring loop
	m.wg.Add(1)
	go m.monitoringLoop(ctx)

	m.running = true
	m.logger.Info("Connectivity monitoring started...")

	return nil
}

// monitoringLoop runs periodic connectivity checks
func (m *Monitor) monitoringLoop(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.checkInterval) // TODO: prefer Timer
	defer ticker.Stop()

	// Run initial check
	m.runConnectivityChecks(ctx)

	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			return
		case <-ticker.C:
			m.runConnectivityChecks(ctx)
		}
	}
}

// runConnectivityChecks performs all connectivity tests
func (m *Monitor) runConnectivityChecks(ctx context.Context) {
	// Run ping tests
	pingResults := make([]*model.PingResult, len(m.pingTargets))
	var wgPing, wgDns sync.WaitGroup
	for i, target := range m.pingTargets {
		wgPing.Add(1)

		go func(n int, t string) {
			defer wgPing.Done()
			pingResults[n] = m.pingTest(t)
		}(i, target) // TODO: explicitly passing variables to goroutine not necessary anymore
	}

	// Run DNS tests
	dnsResults := make([]*model.DNSResult, len(m.dnsTargets))
	for i, target := range m.dnsTargets {
		wgDns.Add(1)

		go func(n int, t string) {
			defer wgDns.Done()
			dnsResults[n] = m.dnsTest(ctx, t)
		}(i, target)
	}
	wgPing.Wait()
	wgDns.Wait()

	// Run gateway test
	gatewayResult := m.gatewayTest()

	m.mu.Lock()
	// Update status (first result only for simplicity)
	if len(pingResults) > 0 { // TODO: Simplicity but this is wrong, maybe summary is best option, with err count
		m.status.Ping = pingResults[0]
	}
	if len(dnsResults) > 0 {
		m.status.DNS = dnsResults[0]
	}
	m.status.Gateway = gatewayResult

	m.status.LastChecked = time.Now()
	m.mu.Unlock()

	// Calculate overall score and status
	m.calculateOverallStatus()
}

// pingTest performs ICMP ping test by promehetus-pinger.
func (m *Monitor) pingTest(target string) *model.PingResult { // TODO we don't have to return pointer

	pinger, err := probing.NewPinger(target)
	if err != nil {
		// bad target
		m.logger.Error("Failed to create pinger", zap.String("target", target), zap.Error(err))
		return &model.PingResult{
			Target:     target,
			RTT:        0,
			Success:    false,
			PacketLoss: 100.0,
		}
	}

	pinger.SetPrivileged(true)
	pinger.Count = 1 // maybe we can set in config.yaml
	pinger.Timeout = m.timeout
	start := time.Now()

	result := &model.PingResult{
		Target:     target,
		Success:    false,
		PacketLoss: 100.0,
	}

	err = pinger.Run()
	result.RTT = time.Since(start)
	if err != nil {
		m.logger.Error("Failed to run pinger", zap.String("target", target), zap.Error(err))
		return result
	}
	stats := pinger.Statistics()

	if stats.PacketsRecv > 0 {
		result.Success = true
		result.PacketLoss = stats.PacketLoss
	} else {
		result.Success = false
		result.PacketLoss = 100.0
	}

	return result
}

// dnsTest performs a DNS resolution test
func (m *Monitor) dnsTest(ctx context.Context, target string) *model.DNSResult { // TODO return by value for less overhead on GC but i know we share the address in the next step
	totalStart := time.Now()

	results := make(map[string]struct{})
	var overallSuccess bool
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, server := range m.dnsServers {
		for _, recordType := range m.recordTypes {
			wg.Add(1)

			go func(s, rt string) {
				defer wg.Done()

				ctxGo, cancel := context.WithTimeout(ctx, m.timeout)
				defer cancel()

				var r *net.Resolver
				if s == "system" {
					r = net.DefaultResolver
				} else {
					r = &net.Resolver{ // TODO: it would be better to reuse resolver in the next run instead of creating from scratch
						PreferGo: true, // use go dns client
						Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
							d := net.Dialer{
								Timeout: m.timeout,
							}
							// could also add logic for TCP.
							return d.DialContext(ctx, "udp", s)
						},
					}
				}

				var records []string
				var err error

				switch rt {
				case "A":
					ips, lookupErr := r.LookupIP(ctxGo, "ip4", target)
					err = lookupErr
					for _, ip := range ips {
						records = append(records, ip.String())
					}
				case "AAAA":
					ips, lookupErr := r.LookupIP(ctxGo, "ip6", target)
					err = lookupErr
					for _, ip := range ips {
						records = append(records, ip.String())
					}
				case "MX":
					mxs, lookupErr := r.LookupMX(ctxGo, target)
					err = lookupErr
					for _, mx := range mxs {
						records = append(records, fmt.Sprintf("%d %s", mx.Pref, mx.Host))
					}
				case "CNAME":
					cname, lookupErr := r.LookupCNAME(ctxGo, target)
					err = lookupErr
					if cname != "" {
						records = append(records, cname)
					}
				case "TXT":
					txts, lookupErr := r.LookupTXT(ctxGo, target)
					err = lookupErr
					for _, txt := range txts {
						records = append(records, txt)
					}
				case "NS":
					nss, lookupErr := r.LookupNS(ctxGo, target)
					err = lookupErr
					for _, ns := range nss {
						records = append(records, ns.Host)
					}
				default:
					err = fmt.Errorf("unsupported record type: %s", rt)
				}

				if err == nil {
					// results map[] shared among goroutnes, best to securely lock it
					mu.Lock()
					overallSuccess = true
					for _, rec := range records {
						if rec != "" {
							results[rec] = struct{}{}
						}
					}
					mu.Unlock()
				} else {
					m.logger.Debug("dns lookup failed",
						zap.String("target", target), zap.String("record_type", rt),
						zap.String("dns_server", s), zap.Error(err))
				}

			}(server, recordType)
		}
	}
	wg.Wait()

	allAddresses := make([]string, 0, len(results))
	for addr := range results {
		allAddresses = append(allAddresses, addr)
	}

	return &model.DNSResult{
		Target:    target,
		Duration:  time.Since(totalStart),
		Success:   overallSuccess,
		Addresses: allAddresses,
	}
}

// gatewayTest performs gateway reachability test
func (m *Monitor) gatewayTest() *model.GatewayResult {
	if m.gateway == "" {
		return &model.GatewayResult{
			Success: false,
		}
	}

	res := m.pingTest(m.gateway)
	return &model.GatewayResult{
		Gateway: m.gateway,
		Success: res.Success,
		RTT:     res.RTT,
	}
}

// discoverGateway finds the default gateway
func (m *Monitor) discoverGateway() error {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	for _, r := range routes {
		if r.Gw == nil {
			continue
		}
		if r.Dst == nil || r.Dst.IP == nil {
			m.gateway = r.Gw.String()
			return nil
		}
		ones, _ := r.Dst.Mask.Size() // 0.0.0.0/0 valid IP.net object
		if ones == 0 {
			m.gateway = r.Gw.String()
			return nil
		}
	}
	m.logger.Warn("no default gateway found, using fallback address", zap.String("fallback gw address", m.gateway))
	return nil
}

// calculateOverallStatus determines overall connectivity status
func (m *Monitor) calculateOverallStatus() { // TODO a better scoring mechanism
	m.mu.Lock()
	defer m.mu.Unlock()

	score := 0
	if m.status.Ping != nil && m.status.Ping.Success {
		score += 30
	}
	if m.status.DNS != nil && m.status.DNS.Success {
		score += 30
	}
	if m.status.Gateway != nil && m.status.Gateway.Success {
		score += 40
	}

	m.status.Score = score
	switch {
	case score >= 90:
		m.status.Overall = "excellent"
	case score >= 70:
		m.status.Overall = "good"
	case score >= 50:
		m.status.Overall = "fair"
	case score >= 30:
		m.status.Overall = "poor"
	default:
		m.status.Overall = "failed"
	}
}

// GetStats returns current connectivity status
func (m *Monitor) GetStats() *model.ConnectivityStats {
	m.mu.RLock()
	s := *m.status // this should suffice since underlying ref values supposed to mutate
	m.mu.RUnlock()
	return &s
}

// UpdateStats sets latest connectivity status
func (m *Monitor) UpdateStats(stats *model.SystemStats) error {
	s := m.GetStats()
	metric.ComponentHealthLastChecked.WithLabelValues(Name).Set(float64(s.LastChecked.Unix())) // TODO instead of calling WithLabelValues everytime, cache it here and reuse
	metric.ComponentHealthStatus.WithLabelValues(Name).Set(metric.HealthStatusToFloat(metric.HealthStatus(s.Overall)))
	stats.Connectivity = s
	return nil
}

// Stop stops the connectivity monitor
func (m *Monitor) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.running = false
	m.logger.Info("Connectivity monitor stopped")
	return nil
}
