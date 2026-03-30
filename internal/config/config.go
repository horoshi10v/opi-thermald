package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
	DataDir             string `json:"data_dir"`
	TelegramBotToken    string `json:"telegram_bot_token"`
	TelegramChatID      string `json:"telegram_chat_id"`
	HostAlias           string `json:"host_alias"`

	Temperature TemperatureConfig `json:"temperature"`
	Storage     StorageConfig     `json:"storage"`
	Summary     SummaryConfig     `json:"summary"`
}

type TemperatureConfig struct {
	SensorPath          string `json:"sensor_path"`
	WarnMilliC          int    `json:"warn_millic"`
	CriticalMilliC      int    `json:"critical_millic"`
	RecoverMilliC       int    `json:"recover_millic"`
	AlertCooldownMinute int    `json:"alert_cooldown_minutes"`
}

type SummaryConfig struct {
	DailyHour    int `json:"daily_hour"`
	DailyMinute  int `json:"daily_minute"`
	WeeklyISO    int `json:"weekly_iso_weekday"`
	WeeklyHour   int `json:"weekly_hour"`
	WeeklyMinute int `json:"weekly_minute"`
}

type StorageConfig struct {
	SampleRetentionDays int `json:"sample_retention_days"`
}

func Load(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}

	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = 30
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "/var/lib/opi-thermald"
	}
	if cfg.Temperature.SensorPath == "" {
		cfg.Temperature.SensorPath = "/sys/class/thermal/thermal_zone0/temp"
	}
	if cfg.Temperature.AlertCooldownMinute <= 0 {
		cfg.Temperature.AlertCooldownMinute = 30
	}
	if cfg.Summary.WeeklyISO < 1 || cfg.Summary.WeeklyISO > 7 {
		cfg.Summary.WeeklyISO = 7
	}
	if cfg.Storage.SampleRetentionDays <= 0 {
		cfg.Storage.SampleRetentionDays = 8
	}
	if cfg.HostAlias == "" {
		host, _ := os.Hostname()
		cfg.HostAlias = host
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return cfg, fmt.Errorf("create data dir: %w", err)
	}

	cfg.DataDir = filepath.Clean(cfg.DataDir)
	return cfg, nil
}

func (c Config) PollInterval() time.Duration {
	return time.Duration(c.PollIntervalSeconds) * time.Second
}

func (c Config) AlertCooldown() time.Duration {
	return time.Duration(c.Temperature.AlertCooldownMinute) * time.Minute
}

func (c Config) SampleRetention() time.Duration {
	return time.Duration(c.Storage.SampleRetentionDays) * 24 * time.Hour
}
