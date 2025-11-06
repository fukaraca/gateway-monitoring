package tests

import (
	"context"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/fukaraca/gateway-monitoring/internal/config"
	"github.com/fukaraca/gateway-monitoring/internal/monitor"
	netlink_pkg "github.com/fukaraca/gateway-monitoring/internal/netlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

func Test_Main(t *testing.T) {
	ctx := context.Background()
	cfg := config.NewConfig("testing")
	require.NoError(t, cfg.Load("test-config.yaml", "./"))
	cfg.Logger = zap.NewNop()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	defer cfg.Logger.Sync()
	now := time.Now().Add(2 * time.Second)
	gw := monitor.NewGatewayMonitor(cfg)

	require.NoError(t, gw.Start(ctx))
	defer func(tst *testing.T) {
		require.NoError(t, gw.Stop())
	}(t)

	assert.Eventually(t, func() bool {
		s := gw.GetStats()
		ok := true
		if cfg.Components.ProcMonitor {
			ok = ok && s.Proc != nil
		}
		if cfg.Components.ConnectivityMonitor {
			ok = ok && s.Connectivity != nil && s.Connectivity.LastChecked.After(now)
		}
		if cfg.Components.NetlinkMonitor {
			ok = ok && s.Netlink != nil && !s.Netlink.LastChecked.IsZero()
		}
		return s.Timestamp.After(now) && ok
	}, time.Second*10, 2*time.Second)

	require.True(t, gw.GetStats().Timestamp.After(now))
	stats := gw.GetStats()
	require.NotNil(t, stats)

	// Verify proc monitor
	if cfg.Components.ProcMonitor {
		require.NotNil(t, stats.Proc)
		require.NotNil(t, stats.Proc.Memory)
		require.Greater(t, stats.Proc.Memory.MemTotal, uint64(0))
		require.NotNil(t, stats.Proc.Load)
		require.Greater(t, stats.Proc.Load.Load1, 0.01)
	}

	// Verify Connectivity
	if cfg.Components.ConnectivityMonitor {
		require.NotEqual(t, stats.Connectivity.Overall, "unknown")
		require.NotNil(t, stats.Connectivity.Ping)
		require.Equal(t, stats.Connectivity.Ping.Target, "8.8.8.8")
		require.NotNil(t, stats.Connectivity.DNS)
		require.True(t, stats.Connectivity.DNS.Success)
		require.NotNil(t, stats.Connectivity.Gateway)
	}

	// Verify Netlink
	if cfg.Components.NetlinkMonitor {
		require.Greater(t, len(stats.Netlink.Interfaces), 0)
		require.Greater(t, len(stats.Netlink.Routes), 0)
		require.Greater(t, len(stats.Netlink.Addresses), 0)

		// test "sudo ip link add name test-dummy0 type dummy"
		snapshotTime := time.Now()

		dummyLinkName := "test-dummy0"
		_, err := netlink.LinkByName(dummyLinkName) // shouldn't exist
		require.Error(t, err)

		dummyLink := &netlink.Dummy{
			LinkAttrs: netlink.LinkAttrs{
				Name: dummyLinkName,
			},
		}
		require.NoError(t, netlink.LinkAdd(dummyLink))
		_, err = netlink.LinkByName(dummyLinkName) // shouldn't exist
		require.NoError(t, err)
		// make sure latest change reflected to stats collector
		assert.Eventually(t, func() bool {
			s := gw.GetStats()
			return s.Netlink.LastChecked.After(snapshotTime)
		}, time.Second*10, 2*time.Second)

		stats = gw.GetStats()
		var found bool
		for _, link := range stats.Netlink.Interfaces {
			if link.Name == dummyLinkName {
				found = true
				break
			}
		}
		require.True(t, found)

		mon, err := gw.GetMonitor(netlink_pkg.Name)
		require.NoError(t, err)
		require.NotNil(t, mon)
		netmon := mon.(*netlink_pkg.Monitor)

		ifaces, err := netmon.GetInterfaces()
		require.NoError(t, err)
		require.Greater(t, len(ifaces), 0)

		routes, err := netmon.GetRoutes()
		require.NoError(t, err)
		require.Greater(t, len(routes), 0)

	}

}
