package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/horoshi10v/opi-thermald/internal/collector"
	"github.com/horoshi10v/opi-thermald/internal/config"
	"github.com/horoshi10v/opi-thermald/internal/storage"
	"github.com/horoshi10v/opi-thermald/internal/telegram"
)

type Service struct {
	cfg       config.Config
	collector *collector.Collector
	store     *storage.Store
	telegram  *telegram.Client
	state     storage.State
}

type summarySpec struct {
	label       string
	window      time.Duration
	bucketCount int
}

var (
	dailySummarySpec = summarySpec{
		label:       "daily",
		window:      24 * time.Hour,
		bucketCount: 24,
	}
	weeklySummarySpec = summarySpec{
		label:       "weekly",
		window:      7 * 24 * time.Hour,
		bucketCount: 28,
	}
)

func New(cfg config.Config) (*Service, error) {
	store := storage.New(cfg.DataDir)
	state, err := store.LoadState()
	if err != nil {
		return nil, err
	}

	return &Service{
		cfg:       cfg,
		collector: collector.New(cfg.Temperature.SensorPath),
		store:     store,
		telegram:  telegram.New(cfg.TelegramBotToken, cfg.TelegramChatID),
		state:     state,
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.PollInterval())
	defer ticker.Stop()

	if _, err := s.collector.Collect(); err != nil {
		log.Printf("initial collect failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.tick(); err != nil {
				log.Printf("tick failed: %v", err)
			}
		}
	}
}

func (s *Service) tick() error {
	sample, err := s.collector.Collect()
	if err != nil {
		return fmt.Errorf("collect sample: %w", err)
	}

	if err := s.store.AppendSample(sample); err != nil {
		return fmt.Errorf("append sample: %w", err)
	}
	if err := s.store.PruneSamples(time.Now().Add(-s.cfg.SampleRetention())); err != nil {
		return fmt.Errorf("prune samples: %w", err)
	}

	if err := s.handleAlerts(sample); err != nil {
		log.Printf("alerts failed: %v", err)
	}
	if err := s.handleSummaries(sample.Timestamp); err != nil {
		log.Printf("summaries failed: %v", err)
	}
	if err := s.handleCommands(sample); err != nil {
		log.Printf("commands failed: %v", err)
	}

	if err := s.store.SaveState(s.state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	return nil
}
