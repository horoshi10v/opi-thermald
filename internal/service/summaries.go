package service

import (
	"fmt"
	"time"

	"github.com/horoshi10v/opi-thermald/internal/collector"
)

type summaryStats struct {
	tempMin        int
	tempMax        int
	tempAvg        float64
	cpuAvg         float64
	cpuMax         float64
	loadAvg        float64
	loadMax        float64
	aboveWarnCount int
	sampleCount    int
	tempSeries     []float64
	cpuSeries      []float64
	loadSeries     []float64
}

func (s *Service) handleSummaries(now time.Time) error {
	if s.shouldSendDaily(now) {
		if err := s.sendSummary(now, dailySummarySpec); err != nil {
			return err
		}
		s.state.LastDailySentAt = now
	}

	if s.shouldSendWeekly(now) {
		if err := s.sendSummary(now, weeklySummarySpec); err != nil {
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

func (s *Service) sendSummary(now time.Time, spec summarySpec) error {
	samples, err := s.store.SamplesSince(now.Add(-spec.window))
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		return nil
	}

	stats := s.computeSummaryStats(samples, spec.bucketCount)
	if err := s.exportSummaryCSV(spec.label, samples); err != nil {
		return err
	}

	caption := s.formatSummaryCaption(spec.label, stats)
	chart, err := renderSummaryChart(
		fmt.Sprintf("%s %s summary", s.cfg.HostAlias, spec.label),
		spec.label,
		stats.tempSeries,
		stats.cpuSeries,
		stats.loadSeries,
	)
	if err != nil {
		return err
	}

	return s.telegram.SendPhoto(fmt.Sprintf("%s-summary.png", spec.label), caption, chart)
}

func (s *Service) computeSummaryStats(samples []collector.Sample, bucketCount int) summaryStats {
	stats := summaryStats{
		tempMin:     samples[0].TempMilliC,
		tempMax:     samples[0].TempMilliC,
		sampleCount: len(samples),
	}

	var tempSum, cpuSum, loadSum float64
	for _, sample := range samples {
		if sample.TempMilliC < stats.tempMin {
			stats.tempMin = sample.TempMilliC
		}
		if sample.TempMilliC > stats.tempMax {
			stats.tempMax = sample.TempMilliC
		}
		tempSum += float64(sample.TempMilliC)
		cpuSum += sample.CPUPercent
		loadSum += sample.Load1
		if sample.CPUPercent > stats.cpuMax {
			stats.cpuMax = sample.CPUPercent
		}
		if sample.Load1 > stats.loadMax {
			stats.loadMax = sample.Load1
		}
		if sample.TempMilliC >= s.cfg.Temperature.WarnMilliC {
			stats.aboveWarnCount++
		}
	}

	stats.tempAvg = (tempSum / float64(len(samples))) / 1000
	stats.cpuAvg = cpuSum / float64(len(samples))
	stats.loadAvg = loadSum / float64(len(samples))
	stats.tempSeries = bucketize(samples, bucketCount, func(sample collector.Sample) float64 {
		return float64(sample.TempMilliC) / 1000
	})
	stats.cpuSeries = bucketize(samples, bucketCount, func(sample collector.Sample) float64 {
		return sample.CPUPercent
	})
	stats.loadSeries = bucketize(samples, bucketCount, func(sample collector.Sample) float64 {
		return sample.Load1
	})

	return stats
}

func (s *Service) formatSummaryCaption(label string, stats summaryStats) string {
	return fmt.Sprintf(
		"%s %s summary\nTemp min/avg/max: %.1f/%.1f/%.1fC\nCPU avg/max: %.2f/%.2f%%\nLoad1 avg/max: %.2f/%.2f\nSamples above warn: %d/%d",
		s.cfg.HostAlias,
		label,
		float64(stats.tempMin)/1000,
		stats.tempAvg,
		float64(stats.tempMax)/1000,
		stats.cpuAvg,
		stats.cpuMax,
		stats.loadAvg,
		stats.loadMax,
		stats.aboveWarnCount,
		stats.sampleCount,
	)
}
