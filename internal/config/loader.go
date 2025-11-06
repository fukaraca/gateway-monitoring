package config

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func NewConfig(version string) *Config {
	return &Config{Version: version}
}

func (c *Config) Load(filename, path string) error {
	v := viper.New()
	v.SetConfigName(filename)
	v.AddConfigPath(path)
	v.SetConfigType("yml")
	v.AutomaticEnv()

	err := v.ReadInConfig()
	if err != nil {
		return err
	}

	err = v.Unmarshal(c)
	if err != nil {
		return err
	}
	c.Logger, err = getLogger(c.Logging, c.Version)

	return err
}

// getLogger spawns simple logger by the config
func getLogger(cfg *LoggingConfig, version string) (*zap.Logger, error) {
	lvl, err := zap.ParseAtomicLevel(cfg.Level)
	if err != nil {
		lvl = zap.NewAtomicLevel() // fallback to level info
	}
	config := zap.Config{
		Encoding:      cfg.Format,
		Level:         lvl,
		OutputPaths:   []string{cfg.Output},
		InitialFields: map[string]interface{}{"version": version},
		EncoderConfig: zap.NewDevelopmentEncoderConfig(), // TODO more granular config
	}
	logger, err := config.Build()
	if err != nil {
		return nil, err
	}
	return logger, nil
}
