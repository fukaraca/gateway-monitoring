package netlink

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/fukaraca/gateway-monitoring/internal/config"
	"github.com/fukaraca/gateway-monitoring/internal/metric"
	"github.com/fukaraca/gateway-monitoring/internal/model"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const Name = "netlink"

// Monitor handles netlink-based network monitoring
type Monitor struct {
	logger *zap.Logger

	linkChan    chan netlink.LinkUpdate
	routeChan   chan netlink.RouteUpdate
	addressChan chan netlink.AddrUpdate

	mu            sync.RWMutex
	wg            sync.WaitGroup
	running       bool
	checkInterval time.Duration
	cancel        context.CancelFunc
	stats         *model.NetlinkStats
}

// NewMonitor creates a new netlink monitor
func NewMonitor(cfg *config.NetlinkConfig, logger *zap.Logger) *Monitor {
	return &Monitor{
		stats: &model.NetlinkStats{
			Interfaces: make(map[int]*model.InterfaceInfo),
		},
		logger:        logger,
		linkChan:      make(chan netlink.LinkUpdate, 100),
		routeChan:     make(chan netlink.RouteUpdate, 100),
		addressChan:   make(chan netlink.AddrUpdate, 100),
		checkInterval: cfg.CheckInterval,
	}
}

func (m *Monitor) Name() string {
	return Name
}

// Start initializes the netlink monitor
func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return fmt.Errorf("monitor already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	// Subscribe to link updates
	if err := netlink.LinkSubscribeWithOptions(m.linkChan, ctx.Done(), netlink.LinkSubscribeOptions{
		ListExisting: true, // seed m.stats
		ErrorCallback: func(err error) {
			if err != nil {
				m.logger.Warn("link subscribe error", zap.Error(err))
			}
		},
	}); err != nil {
		return fmt.Errorf("failed to subscribe to link updates: %w", err)
	}

	// Subscribe to route updates
	if err := netlink.RouteSubscribeWithOptions(m.routeChan, ctx.Done(), netlink.RouteSubscribeOptions{
		ListExisting: true,
		ErrorCallback: func(err error) {
			if err != nil {
				m.logger.Warn("route subscribe error", zap.Error(err))
			}
		},
	}); err != nil {
		return fmt.Errorf("failed to subscribe to route updates: %w", err)
	}

	// Subscribe to address changes
	if err := netlink.AddrSubscribeWithOptions(m.addressChan, ctx.Done(), netlink.AddrSubscribeOptions{
		ListExisting: true,
		ErrorCallback: func(err error) {
			if err != nil {
				m.logger.Warn("addr subscribe error", zap.Error(err))
			}
		},
	}); err != nil {
		return fmt.Errorf("failed to subscribe to route updates: %w", err)
	}

	m.wg.Add(1)
	// Start event processing
	go m.processEvents(ctx)

	m.running = true
	m.logger.Info("Netlink monitoring started...")
	return nil
}

// processEvents handles incoming netlink events
func (m *Monitor) processEvents(ctx context.Context) {
	defer m.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-m.linkChan:
			m.applyInterfaceChanges(&update)
		case update := <-m.routeChan:
			m.applyRouteChanges(&update)
		case update := <-m.addressChan:
			m.applyAddressChanges(&update)
		}
	}
}

// GetInterfaces returns current network interface information
func (m *Monitor) GetInterfaces() ([]model.InterfaceInfo, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	out := make([]model.InterfaceInfo, len(links))
	for i, link := range links {
		if link == nil {
			continue
		}
		out[i] = m.linkToInterfaceInfo(link)
	}

	return out, err
}

// applyInterfaceChanges updates interface change
func (m *Monitor) applyInterfaceChanges(link *netlink.LinkUpdate) {
	m.logger.Debug("network interface change detected", zap.String("name", link.Link.Attrs().Name))
	iInfo := m.linkToInterfaceInfo(link.Link)
	m.mu.Lock()
	m.stats.Interfaces[link.Attrs().Index] = &iInfo
	m.stats.LastChecked = time.Now()
	m.mu.Unlock()
}

// applyRouteChanges updates route change
func (m *Monitor) applyRouteChanges(route *netlink.RouteUpdate) {
	m.logger.Debug("route change detected")
	rInfo := m.routeToRouteInfo(route.Route)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats.LastChecked = time.Now()
	if route.Type == unix.RTM_NEWROUTE {
		m.stats.Routes = append(m.stats.Routes, rInfo)
	} else {
		for i, info := range m.stats.Routes {
			if info.Gateway == rInfo.Gateway {
				m.stats.Routes[i] = rInfo
				return
			}
		}
	}
}

// applyAddressChanges updates address list as change suggests
func (m *Monitor) applyAddressChanges(addr *netlink.AddrUpdate) {
	m.logger.Debug("address change detected")
	aInfo := m.addrUpdateToAddressInfo(*addr)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats.LastChecked = time.Now()
	if addr.NewAddr {
		m.stats.Addresses = append(m.stats.Addresses, aInfo)
	} else {
		for i, a := range m.stats.Addresses {
			if a.LinkAddress == aInfo.LinkAddress {
				m.stats.Addresses[i] = aInfo
				return
			}
		}
	}
}

// GetRoutes returns current routing table information
func (m *Monitor) GetRoutes() ([]model.RouteInfo, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return nil, err
	}
	out := make([]model.RouteInfo, len(routes))
	for i, route := range routes {
		out[i] = m.routeToRouteInfo(route)
	}
	return out, nil
}

func (m *Monitor) GetStats() *model.NetlinkStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s := model.NetlinkStats{
		LastChecked: m.stats.LastChecked,
	}

	if m.stats.Interfaces != nil {
		s.Interfaces = make(map[int]*model.InterfaceInfo, len(m.stats.Interfaces))
		for k, v := range m.stats.Interfaces {
			s.Interfaces[k] = v
		}
	}

	if m.stats.Routes != nil {
		s.Routes = make([]model.RouteInfo, len(m.stats.Routes))
		copy(s.Routes, m.stats.Routes)
	}

	if m.stats.Addresses != nil {
		s.Addresses = make([]model.AddressInfo, len(m.stats.Addresses))
		copy(s.Addresses, m.stats.Addresses)
	}
	return &s
}

// UpdateStats sets updated netlink stats
func (m *Monitor) UpdateStats(stats *model.SystemStats) error {
	stats.Netlink = m.GetStats()

	metric.ComponentHealthLastChecked.WithLabelValues(Name).SetToCurrentTime()
	metric.ComponentHealthStatus.WithLabelValues(Name).Set(10)

	return nil
}

// Stop closes the netlink socket (missing proper cleanup)
func (m *Monitor) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}

	m.wg.Wait()
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()

	m.logger.Info("Netlink monitor stopped")
	return nil
}

// linkToInterfaceInfo converts netlink.Link to model.InterfaceInfo
func (m *Monitor) linkToInterfaceInfo(link netlink.Link) model.InterfaceInfo {
	var addresses []string
	addr, errInner := netlink.AddrList(link, netlink.FAMILY_ALL)
	if errInner != nil {
		m.logger.Debug("address couldn't read for link", zap.String("link_name", link.Attrs().Name), zap.Error(errInner))
	} else {
		for _, a := range addr {
			addresses = append(addresses, a.String())
		}
	}

	iInfo := model.InterfaceInfo{
		Name:         link.Attrs().Name,
		Index:        link.Attrs().Index,
		MTU:          link.Attrs().MTU,
		HardwareAddr: link.Attrs().HardwareAddr.String(),
		Flags:        parseInterfaceFlags(link.Attrs().Flags),
		State:        link.Attrs().OperState.String(),
		Addresses:    addresses,
	}
	return iInfo
}

// routeToRouteInfo converts netlink.Route to model.RouteInfo
func (m *Monitor) routeToRouteInfo(route netlink.Route) model.RouteInfo {
	dst := "nil"
	gw := "nil"
	if route.Dst != nil {
		dst = route.Dst.String()
	}
	if !route.Gw.Equal(net.IP{}) {
		gw = route.Gw.String()
	}
	return model.RouteInfo{
		Destination:    dst,
		Gateway:        gw,
		InterfaceIndex: route.LinkIndex,
		Metric:         route.Priority,
	}
}

// addrUpdateToAddressInfo converts netlink.AddrUpdate to model.AddressInfo
func (m *Monitor) addrUpdateToAddressInfo(addr netlink.AddrUpdate) model.AddressInfo {
	return model.AddressInfo{
		LinkAddress: addr.LinkAddress.String(),
		LinkIndex:   addr.LinkIndex,
		Flags:       addr.Flags,
	}
}

// parseInterfaceFlags converts netlink flags to string representation
func parseInterfaceFlags(flags net.Flags) []string {
	var flagList []string

	if flags&net.FlagUp != 0 {
		flagList = append(flagList, "UP")
	}
	if flags&net.FlagBroadcast != 0 {
		flagList = append(flagList, "BROADCAST")
	}
	if flags&net.FlagLoopback != 0 {
		flagList = append(flagList, "LOOPBACK")
	}
	if flags&net.FlagPointToPoint != 0 {
		flagList = append(flagList, "POINTOPOINT")
	}
	if flags&net.FlagMulticast != 0 {
		flagList = append(flagList, "MULTICAST")
	}

	return flagList
}
