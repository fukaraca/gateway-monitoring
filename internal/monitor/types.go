package monitor

import (
	"context"

	"github.com/fukaraca/gateway-monitoring/internal/model"
)

// Monitor defines the interface for all monitoring components
type Monitor interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
	UpdateStats(stats *model.SystemStats) error
}
