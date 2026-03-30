package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/valentyn-khoroshylov/opi-thermald/internal/collector"
	"github.com/valentyn-khoroshylov/opi-thermald/internal/config"
	"github.com/valentyn-khoroshylov/opi-thermald/internal/storage"
	"github.com/valentyn-khoroshylov/opi-thermald/internal/telegram"
)

type Service struct {
	cfg       config.Config
	collector *collector.Collector
	store     *storage.Store
	telegram  *telegram.Client
	state     storage.State
}

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
		return err
	}

	if err := s.store.AppendSample(sample); err != nil {
		return err
	}
	if err := s.store.PruneSamples(time.Now().Add(-8 * 24 * time.Hour)); err != nil {
		return err
	}

	if err := s.handleAlerts(sample); err != nil {
		log.Printf("alerts failed: %v", err)
	}
	if err := s.handleSummaries(sample.Timestamp); err != nil {
		log.Printf("summaries failed: %v", err)
	}

	return s.store.SaveState(s.state)
}

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

func (s *Service) handleSummaries(now time.Time) error {
	if s.shouldSendDaily(now) {
		if err := s.sendSummary(now, 24*time.Hour, "daily"); err != nil {
			return err
		}
		s.state.LastDailySentAt = now
	}

	if s.shouldSendWeekly(now) {
		if err := s.sendSummary(now, 7*24*time.Hour, "weekly"); err != nil {
			return err
		}
		s.state.LastWeeklySentAt = now
	}
	return nil
}

func (s *Service) shouldSendDaily(now time.Time) bool {
	if now.Hour() != s.cfg.Summary.DailyHour || now.Minute() != s.cfg.Summary.DailyMinute {
		return false
	}
	last := s.state.LastDailySentAt
	return last.IsZero() || last.YearDay() != now.YearDay() || last.Year() != now.Year()
}

func (s *Service) shouldSendWeekly(now time.Time) bool {
	if int(now.Weekday()) == 0 {
		if s.cfg.Summary.WeeklyISO != 7 {
			return false
		}
	} else if int(now.Weekday()) != s.cfg.Summary.WeeklyISO {
		return false
	}

	if now.Hour() != s.cfg.Summary.WeeklyHour || now.Minute() != s.cfg.Summary.WeeklyMinute {
		return false
	}

	last := s.state.LastWeeklySentAt
	if last.IsZero() {
		return true
	}

	yearNow, weekNow := now.ISOWeek()
	yearLast, weekLast := last.ISOWeek()
	return yearNow != yearLast || weekNow != weekLast
}

func (s *Service) sendSummary(now time.Time, window time.Duration, label string) error {
	samples, err := s.store.SamplesSince(now.Add(-window))
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		return nil
	}

	tempMin := samples[0].TempMilliC
	tempMax := samples[0].TempMilliC
	var tempSum, cpuSum, loadSum float64
	var cpuMax, loadMax float64
	var aboveWarnCount int

	for _, sample := range samples {
		if sample.TempMilliC < tempMin {
			tempMin = sample.TempMilliC
		}
		if sample.TempMilliC > tempMax {
			tempMax = sample.TempMilliC
		}
		tempSum += float64(sample.TempMilliC)
		cpuSum += sample.CPUPercent
		loadSum += sample.Load1
		if sample.CPUPercent > cpuMax {
			cpuMax = sample.CPUPercent
		}
		if sample.Load1 > loadMax {
			loadMax = sample.Load1
		}
		if sample.TempMilliC >= s.cfg.Temperature.WarnMilliC {
			aboveWarnCount++
		}
	}

	tempSpark := sparkline(bucketize(samples, 24, func(sample collector.Sample) float64 {
		return float64(sample.TempMilliC) / 1000
	}))
	cpuSpark := sparkline(bucketize(samples, 24, func(sample collector.Sample) float64 {
		return sample.CPUPercent
	}))
	loadSpark := sparkline(bucketize(samples, 24, func(sample collector.Sample) float64 {
		return sample.Load1
	}))

	msg := fmt.Sprintf(
		"%s %s summary\nTemp min/avg/max: %.1f/%.1f/%.1fC\nTemp: %s\nCPU avg/max: %.2f/%.2f%%\nCPU:  %s\nLoad1 avg/max: %.2f/%.2f\nLoad: %s\nSamples above warn: %d/%d",
		s.cfg.HostAlias,
		label,
		float64(tempMin)/1000,
		(tempSum/float64(len(samples)))/1000,
		float64(tempMax)/1000,
		tempSpark,
		cpuSum/float64(len(samples)),
		cpuMax,
		cpuSpark,
		loadSum/float64(len(samples)),
		loadMax,
		loadSpark,
		aboveWarnCount,
		len(samples),
	)

	return s.telegram.Send(msg)
}

func bucketize(samples []collector.Sample, bucketCount int, valueFn func(collector.Sample) float64) []float64 {
	if len(samples) == 0 || bucketCount <= 0 {
		return nil
	}

	buckets := make([][]float64, bucketCount)
	start := samples[0].Timestamp
	end := samples[len(samples)-1].Timestamp
	span := end.Sub(start)
	if span <= 0 {
		span = time.Second
	}

	for _, sample := range samples {
		offset := sample.Timestamp.Sub(start)
		index := int((float64(offset) / float64(span)) * float64(bucketCount))
		if index >= bucketCount {
			index = bucketCount - 1
		}
		buckets[index] = append(buckets[index], valueFn(sample))
	}

	result := make([]float64, 0, bucketCount)
	var last float64
	var haveLast bool
	for _, bucket := range buckets {
		if len(bucket) == 0 {
			if haveLast {
				result = append(result, last)
			} else {
				result = append(result, 0)
			}
			continue
		}

		var sum float64
		for _, value := range bucket {
			sum += value
		}
		last = sum / float64(len(bucket))
		haveLast = true
		result = append(result, last)
	}

	return result
}

func sparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}

	const glyphs = "▁▂▃▄▅▆▇█"

	minVal := values[0]
	maxVal := values[0]
	for _, value := range values[1:] {
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
	}

	var b strings.Builder
	if maxVal == minVal {
		for range values {
			b.WriteRune('▁')
		}
		return b.String()
	}

	scale := float64(len(glyphs) - 1)
	for _, value := range values {
		index := int(math.Round(((value - minVal) / (maxVal - minVal)) * scale))
		if index < 0 {
			index = 0
		}
		if index >= len(glyphs) {
			index = len(glyphs) - 1
		}
		b.WriteByte(glyphs[index])
	}

	return b.String()
}
