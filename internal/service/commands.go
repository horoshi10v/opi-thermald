package service

import (
	"fmt"
	"strings"

	"github.com/horoshi10v/opi-thermald/internal/collector"
)

const (
	commandTemp    = "/temp"
	commandStatus  = "/status"
	commandSummary = "/summary"
	commandWeekly  = "/weekly"
)

func (s *Service) handleCommands(sample collector.Sample) error {
	updates, err := s.telegram.GetUpdates(s.state.LastUpdateID + 1)
	if err != nil {
		return err
	}

	for _, update := range updates {
		if update.UpdateID > s.state.LastUpdateID {
			s.state.LastUpdateID = update.UpdateID
		}

		if update.Message.Text == "" {
			continue
		}
		if fmt.Sprintf("%d", update.Message.Chat.ID) != s.cfg.TelegramChatID {
			continue
		}

		switch strings.TrimSpace(update.Message.Text) {
		case commandTemp:
			if err := s.telegram.Send(s.formatTempMessage(sample)); err != nil {
				return err
			}
		case commandStatus:
			if err := s.telegram.Send(s.formatStatusMessage(sample)); err != nil {
				return err
			}
		case commandSummary:
			if err := s.sendSummary(sample.Timestamp, dailySummarySpec); err != nil {
				return err
			}
		case commandWeekly:
			if err := s.sendSummary(sample.Timestamp, weeklySummarySpec); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Service) formatTempMessage(sample collector.Sample) string {
	return fmt.Sprintf(
		"%s current temperature: %.1fC",
		s.cfg.HostAlias,
		float64(sample.TempMilliC)/1000,
	)
}

func (s *Service) formatStatusMessage(sample collector.Sample) string {
	return fmt.Sprintf(
		"%s status\nTemp: %.1fC\nCPU: %.2f%%\nLoad1: %.2f\nMem: %.2f%%\nDisk: %.2f%%",
		s.cfg.HostAlias,
		float64(sample.TempMilliC)/1000,
		sample.CPUPercent,
		sample.Load1,
		sample.MemUsedPct,
		sample.DiskUsedPct,
	)
}
