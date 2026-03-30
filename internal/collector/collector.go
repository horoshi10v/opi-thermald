package collector

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Sample struct {
	Timestamp   time.Time `json:"timestamp"`
	TempMilliC  int       `json:"temp_millic"`
	CPUPercent  float64   `json:"cpu_percent"`
	Load1       float64   `json:"load1"`
	MemUsedPct  float64   `json:"mem_used_pct"`
	DiskUsedPct float64   `json:"disk_used_pct"`
}

type Collector struct {
	sensorPath string
	prevTotal  uint64
	prevIdle   uint64
	haveCPURef bool
}

func New(sensorPath string) *Collector {
	return &Collector{sensorPath: sensorPath}
}

func (c *Collector) Collect() (Sample, error) {
	tempMilliC, err := c.readInt(c.sensorPath)
	if err != nil {
		return Sample{}, fmt.Errorf("read temp: %w", err)
	}

	load1, err := c.readLoad1()
	if err != nil {
		return Sample{}, fmt.Errorf("read loadavg: %w", err)
	}

	memUsedPct, err := c.readMemUsedPct()
	if err != nil {
		return Sample{}, fmt.Errorf("read meminfo: %w", err)
	}

	diskUsedPct, err := c.readDiskUsedPct("/")
	if err != nil {
		return Sample{}, fmt.Errorf("read disk usage: %w", err)
	}

	cpuPct, err := c.readCPUPercent()
	if err != nil {
		return Sample{}, fmt.Errorf("read cpu usage: %w", err)
	}

	return Sample{
		Timestamp:   time.Now(),
		TempMilliC:  tempMilliC,
		CPUPercent:  cpuPct,
		Load1:       load1,
		MemUsedPct:  memUsedPct,
		DiskUsedPct: diskUsedPct,
	}, nil
}

func (c *Collector) readInt(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func (c *Collector) readLoad1() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected loadavg format")
	}
	return strconv.ParseFloat(fields[0], 64)
}

func (c *Collector) readMemUsedPct() (float64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var memTotal, memAvailable float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			memTotal, _ = strconv.ParseFloat(fields[1], 64)
		case "MemAvailable:":
			memAvailable, _ = strconv.ParseFloat(fields[1], 64)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if memTotal <= 0 {
		return 0, fmt.Errorf("MemTotal not found")
	}
	usedPct := ((memTotal - memAvailable) / memTotal) * 100
	return round2(usedPct), nil
}

func (c *Collector) readDiskUsedPct(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	total := float64(stat.Blocks) * float64(stat.Bsize)
	avail := float64(stat.Bavail) * float64(stat.Bsize)
	if total <= 0 {
		return 0, fmt.Errorf("disk total is zero")
	}
	usedPct := ((total - avail) / total) * 100
	return round2(usedPct), nil
}

func (c *Collector) readCPUPercent() (float64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0, fmt.Errorf("empty /proc/stat")
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 8 || fields[0] != "cpu" {
		return 0, fmt.Errorf("unexpected /proc/stat format")
	}

	var values []uint64
	for _, field := range fields[1:] {
		v, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, err
		}
		values = append(values, v)
	}

	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}

	var total uint64
	for _, v := range values {
		total += v
	}

	if !c.haveCPURef {
		c.prevTotal = total
		c.prevIdle = idle
		c.haveCPURef = true
		return 0, nil
	}

	deltaTotal := total - c.prevTotal
	deltaIdle := idle - c.prevIdle
	c.prevTotal = total
	c.prevIdle = idle

	if deltaTotal == 0 {
		return 0, nil
	}

	usedPct := (1 - float64(deltaIdle)/float64(deltaTotal)) * 100
	return round2(usedPct), nil
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
