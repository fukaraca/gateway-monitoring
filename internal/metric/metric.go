package metric

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/fukaraca/gateway-monitoring/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

var namespace = "gwm"

// HealthStatus represents the health state of a component
type HealthStatus string

const (
	HealthStatusUnknown  HealthStatus = "unknown"
	HealthStatusHealthy  HealthStatus = "healthy"
	HealthStatusWarning  HealthStatus = "warning"
	HealthStatusCritical HealthStatus = "critical"
)

// HealthStatusToFloat is helper function to map enum status to a float64 for Prometheus
func HealthStatusToFloat(status HealthStatus) float64 {
	switch status {
	case HealthStatusHealthy, "excellent":
		return 10.0
	case HealthStatusWarning, "good":
		return 7.0
	case HealthStatusCritical, "fair", "poor", "failed":
		return 5.0
	default:
		return 0.0
	}
}

// ComponentHealth represents health information for a system component
type ComponentHealth struct {
	Name        string       `json:"name"`
	Status      HealthStatus `json:"status"`
	LastChecked time.Time    `json:"last_checked"`
	Message     string       `json:"message,omitempty"`
	Metrics     interface{}  `json:"metrics,omitempty"`
}

// SystemHealth aggregates health information from all components
type SystemHealth struct {
	Overall    HealthStatus      `json:"overall"`
	Timestamp  time.Time         `json:"timestamp"`
	Components []ComponentHealth `json:"components"`
	Uptime     time.Duration     `json:"uptime"`
}

var (
	SystemHealthStatus = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "system_health",
		Help:      "Overall system health status (0=unknown, 1=healthy, 2=warning, 3=critical).",
	})

	SystemUptime = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "system_uptime_seconds",
		Help:      "Current system uptime in seconds.",
	})

	SystemHealthLastChecked = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "system_health_last_checked_seconds",
		Help:      "Timestamp of the last completed system health check.",
	})

	ComponentHealthStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "component_health",
		Help:      "Health status of individual components (0=unknown, 1=healthy, 2=warning, 3=critical).",
	},
		[]string{"component"},
	)

	ComponentHealthLastChecked = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "component_health_last_checked_seconds",
		Help:      "Timestamp of the last completed health check for a specific component.",
	},
		[]string{"component"},
	)
)

type Exporter struct {
	logger *zap.Logger
	srv    *http.Server
	cancel context.CancelFunc
	cfg    *config.PrometheusServerConfig
}

func NewExporter(cfg *config.PrometheusServerConfig, logger *zap.Logger) *Exporter {
	namespace = cfg.Namespace
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &Exporter{cfg: cfg, srv: srv, logger: logger}

}

func (e *Exporter) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	errCh := make(chan error, 1)
	go func() {
		e.logger.Info("Prometheus exporter listening", zap.String("addr", e.srv.Addr))
		if err := e.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case <-ctx.Done():
		e.logger.Info("stopping prometheus server")
		// graceful shutdown
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return e.srv.Shutdown(shCtx)
	case err := <-errCh:
		e.logger.Error("prometheus server failed", zap.Error(err))
		return err
	}
}

func (e *Exporter) Stop() error {
	e.cancel()
	return nil
}
