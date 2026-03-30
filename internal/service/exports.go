package service

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/horoshi10v/opi-thermald/internal/collector"
)

func (s *Service) exportSummaryCSV(label string, samples []collector.Sample) error {
	exportDir := filepath.Join(s.cfg.DataDir, "exports")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(exportDir, fmt.Sprintf("%s-latest.csv", label))
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"timestamp",
		"temp_c",
		"cpu_percent",
		"load1",
		"mem_used_pct",
		"disk_used_pct",
	}); err != nil {
		return err
	}

	for _, sample := range samples {
		record := []string{
			sample.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%.3f", float64(sample.TempMilliC)/1000),
			fmt.Sprintf("%.2f", sample.CPUPercent),
			fmt.Sprintf("%.2f", sample.Load1),
			fmt.Sprintf("%.2f", sample.MemUsedPct),
			fmt.Sprintf("%.2f", sample.DiskUsedPct),
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return writer.Error()
}
