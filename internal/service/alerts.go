package service

import (
	"fmt"

	"github.com/horoshi10v/opi-thermald/internal/collector"
)

func (s *Service) handleAlerts(sample collector.Sample) error {
	level := ""
	switch {
	case sample.TempMilliC >= s.cfg.Temperature.CriticalMilliC:
		level = "critical"
	case sample.TempMilliC >= s.cfg.Temperature.WarnMilliC:
		level = "warning"
	}

	now := sample.Timestamp

	if level == "" {
		if s.state.AlertActive && sample.TempMilliC <= s.cfg.Temperature.RecoverMilliC {
			msg := fmt.Sprintf(
				"%s: temperature recovered to %.1fC, CPU %.2f%%, load1 %.2f",
				s.cfg.HostAlias,
				float64(sample.TempMilliC)/1000,
				sample.CPUPercent,
				sample.Load1,
			)
			if err := s.telegram.Send(msg); err != nil {
				return err
			}
			s.state.AlertActive = false
			s.state.AlertLevel = ""
			s.state.LastAlertSentAt = now
		}
		return nil
	}

	shouldSend := !s.state.AlertActive ||
		s.state.AlertLevel != level ||
		now.Sub(s.state.LastAlertSentAt) >= s.cfg.AlertCooldown()
	if !shouldSend {
		return nil
	}

	msg := fmt.Sprintf(
		"%s: %s temperature alert %.1fC, CPU %.2f%%, load1 %.2f, mem %.2f%%, disk %.2f%%",
		s.cfg.HostAlias,
		level,
		float64(sample.TempMilliC)/1000,
		sample.CPUPercent,
		sample.Load1,
		sample.MemUsedPct,
		sample.DiskUsedPct,
	)
	if err := s.telegram.Send(msg); err != nil {
		return err
	}

	s.state.AlertActive = true
	s.state.AlertLevel = level
	s.state.LastAlertSentAt = now
	return nil
}
