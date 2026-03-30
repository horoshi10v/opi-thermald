package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/valentyn-khoroshylov/opi-thermald/internal/collector"
)

type State struct {
	AlertLevel       string    `json:"alert_level"`
	AlertActive      bool      `json:"alert_active"`
	LastAlertSentAt  time.Time `json:"last_alert_sent_at"`
	LastDailySentAt  time.Time `json:"last_daily_sent_at"`
	LastWeeklySentAt time.Time `json:"last_weekly_sent_at"`
	LastUpdateID     int64     `json:"last_update_id"`
}

type Store struct {
	samplesPath string
	statePath   string
}

func New(dataDir string) *Store {
	return &Store{
		samplesPath: filepath.Join(dataDir, "samples.jsonl"),
		statePath:   filepath.Join(dataDir, "state.json"),
	}
}

func (s *Store) AppendSample(sample collector.Sample) error {
	f, err := os.OpenFile(s.samplesPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) LoadState() (State, error) {
	var state State
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func (s *Store) SaveState(state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath, data, 0o644)
}

func (s *Store) SamplesSince(since time.Time) ([]collector.Sample, error) {
	f, err := os.Open(s.samplesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	samples := make([]collector.Sample, 0, 1024)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var sample collector.Sample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			return nil, fmt.Errorf("decode sample: %w", err)
		}
		if sample.Timestamp.Before(since) {
			continue
		}
		samples = append(samples, sample)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func (s *Store) PruneSamples(keepSince time.Time) error {
	f, err := os.Open(s.samplesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	tmpPath := s.samplesPath + ".tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	writer := bufio.NewWriter(tmp)
	for scanner.Scan() {
		var sample collector.Sample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			_ = tmp.Close()
			return err
		}
		if sample.Timestamp.Before(keepSince) {
			continue
		}
		if _, err := writer.Write(append(scanner.Bytes(), '\n')); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := writer.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.samplesPath)
}
