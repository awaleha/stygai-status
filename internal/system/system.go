package system

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

type Stats struct {
	Hostname string             `json:"hostname"`
	CPUPct   float64            `json:"cpu_pct"`
	CPUs     map[string]float64 `json:"cpu_pct_percpu"`
	MemUsed  uint64             `json:"mem_used"`
	MemTotal uint64             `json:"mem_total"`
	Uptime   uint64             `json:"uptime"`
	Main     Disk               `json:"disk_main"`
	Stag     Disk               `json:"disk_stag"`
	StagFree uint64             `json:"disk_stag_free"`
	LoadAvg  Loads              `json:"load_avg"`
}

type Disk struct {
	Path        string  `json:"path"`
	Used        uint64  `json:"used"`
	Total       uint64  `json:"total"`
	UsedPercent float64 `json:"used_percent"`
}

type CpuPercent struct {
	CpuPer []float64
	Err    error
}

type Loads struct {
	Load1  float64 `json:"load_1"`
	Load5  float64 `json:"load_5"`
	Load15 float64 `json:"load_15"`
}

func Collect() (Stats, error) {

	result := make(chan CpuPercent, 1)
	go func() {
		cpuPer, err := cpu.Percent(500*time.Millisecond, true)
		result <- CpuPercent{cpuPer, err}
	}()

	hInfo, err := host.Info()
	if err != nil {
		return Stats{}, fmt.Errorf("failed to get host info: %w", err)
	}

	vMem, err := mem.VirtualMemory()
	if err != nil {
		return Stats{}, fmt.Errorf("failed to get memory info: %w", err)
	}

	mDisk, err := disk.Usage("/")
	if err != nil {
		return Stats{}, fmt.Errorf("failed to get main disk info: %w", err)
	}

	sDisk, err := disk.Usage("/mnt/stag")
	if err != nil {
		return Stats{}, fmt.Errorf("failed to get stag disk info: %w", err)
	}

	loadAvg, err := load.Avg()
	if err != nil {
		return Stats{}, fmt.Errorf("failed to get load info: %w", err)
	}

	cpuPer := <-result //wait for cpu interval to finish
	if cpuPer.Err != nil {
		return Stats{}, fmt.Errorf("failed to get cpu percentage: %w", cpuPer.Err)
	}
	if len(cpuPer.CpuPer) == 0 {
		return Stats{}, fmt.Errorf("no per-cpu samples returned")
	}

	perCpu := make(map[string]float64, len(cpuPer.CpuPer))
	var cpuAvg float64
	for i, p := range cpuPer.CpuPer {
		perCpu["C"+strconv.Itoa(i)] = roundTo2d(p)
		cpuAvg += p
	}
	cpuAvg = cpuAvg / float64(len(cpuPer.CpuPer))

	return Stats{
			Hostname: hInfo.Hostname,
			CPUPct:   roundTo2d(cpuAvg),
			CPUs:     perCpu,
			MemUsed:  vMem.Used,
			MemTotal: vMem.Total,
			Uptime:   hInfo.Uptime,
			Main: Disk{
				Path:        mDisk.Path,
				Used:        mDisk.Used,
				Total:       mDisk.Total,
				UsedPercent: roundTo2d(mDisk.UsedPercent),
			},
			Stag: Disk{
				Path:        sDisk.Path,
				Used:        sDisk.Used,
				Total:       sDisk.Total,
				UsedPercent: roundTo2d(sDisk.UsedPercent),
			},
			StagFree: sDisk.Free,
			LoadAvg: Loads{
				Load1:  loadAvg.Load1,
				Load5:  loadAvg.Load5,
				Load15: loadAvg.Load15,
			},
		},
		nil
}

func roundTo2d(num float64) float64 {
	return math.Round(num*100) / 100
}
