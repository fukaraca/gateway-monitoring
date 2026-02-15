package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/fukaraca/gateway-monitoring/internal/config"
	"github.com/fukaraca/gateway-monitoring/internal/monitor"
	"github.com/fukaraca/gateway-monitoring/internal/utils"
	"github.com/spf13/cobra"
	"github.com/syndtr/gocapability/capability"
	"go.uber.org/zap"
)

var (
	configName = ""
	Version    = "dev"
)

func main() {
	// Check if running as root (required for netlink operations)
	if ok, err := utils.CheckCapabilities(capability.CAP_NET_ADMIN, capability.CAP_NET_RAW); !ok {
		log.Fatal("capability check failed", err)
	}

	if err := rootCommand().Execute(); err != nil {
		log.Fatal(err)
	}
}

func rootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gateway-monitor",
		Short: "gateway-monitor",
		RunE: func(cmd *cobra.Command, args []string) error {
			return initialize()
		},
		Version: Version,
	}
	rootCmd.PersistentFlags().StringVar(&configName, "config", "config.yaml", "config file to be used")

	return rootCmd
}

func initialize() error {
	cfg := config.NewConfig(Version)
	err := cfg.Load(configName, "./")
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	defer cfg.Logger.Sync()

	gw := monitor.NewGatewayMonitor(cfg)

	// Start monitoring
	if err = gw.Start(ctx); err != nil {
		cfg.Logger.Error("Failed to start monitor", zap.Error(err))
		return err
	}

	select {
	case <-ctx.Done(): // TODO we don't exit unless signals called
		cfg.Logger.Warn("Received shutdown signal", zap.Error(ctx.Err()))
		err = gw.Stop() // Graceful(ish) shutdown
	}

	return err
}
