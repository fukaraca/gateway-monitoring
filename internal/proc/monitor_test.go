package proc

import (
	"context"
	"testing"
	"time"

	"github.com/fukaraca/gateway-monitoring/internal/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestProcStartStop(t *testing.T) {
	mon := NewMonitor(&config.ProcConfig{
		CheckInterval: time.Second,
		MeminfoPath:   "/proc/meminfo",
		LoadavgPath:   "/proc/loadavg",
		NetdevPath:    "/proc/net/dev",
	}, zap.NewNop())

	require.NoError(t, mon.Start(context.Background()))
	require.Error(t, mon.Start(context.Background())) // should fail

	require.NoError(t, mon.Stop())
	require.NoError(t, mon.Start(context.Background())) // should pass
	require.NoError(t, mon.Stop())
}
