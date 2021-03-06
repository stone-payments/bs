// Copyright 2016 bs authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metric

import (
	"errors"
	"fmt"
	"os"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
	"github.com/tsuru/bs/bslog"
	"github.com/tsuru/bs/config"
)

type HostClient struct {
	ifaceName    string
	lastCPUStats *cpu.CPUTimesStat
}

type errInterfaceNotFound struct {
	name string
}

func (e errInterfaceNotFound) Error() string {
	return fmt.Sprintf("interface %s not found", e.name)
}

func NewHostClient() (*HostClient, error) {
	proc := os.Getenv("HOST_PROC")
	if proc == "" {
		return nil, errors.New("HOST_PROC must be set to be able to send host metrics")
	}
	return &HostClient{
		ifaceName: config.StringEnvOrDefault("eth0", "METRICS_NETWORK_INTERFACE"),
	}, nil
}

func (h *HostClient) GetHostMetrics() ([]map[string]float, error) {
	collectors := []func() (map[string]float, error){
		h.getHostLoad,
		h.getHostMem,
		h.getHostSwap,
		h.getHostFileSystemUsage,
		h.getHostUptime,
		h.getHostCpuTimes,
		h.getHostNetworkUsage,
	}
	var metrics []map[string]float
	for _, collector := range collectors {
		metric, err := collector()
		if err != nil {
			if _, ok := err.(errInterfaceNotFound); ok {
				bslog.Warnf("Skipping network metrics: %s", err)
				continue
			}
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func (h *HostClient) getHostLoad() (map[string]float, error) {
	loadStat, err := load.LoadAvg()
	if err != nil {
		return nil, err
	}
	stats := map[string]float{
		"load1":  float(loadStat.Load1),
		"load5":  float(loadStat.Load5),
		"load15": float(loadStat.Load15),
	}
	return stats, nil
}

func (h *HostClient) getHostMem() (map[string]float, error) {
	memStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	stats := map[string]float{
		"mem_total": float(memStat.Total),
		"mem_used":  float(memStat.Used),
		"mem_free":  float(memStat.Free),
	}
	return stats, nil
}

func (h *HostClient) getHostSwap() (map[string]float, error) {
	swap, err := mem.SwapMemory()
	if err != nil {
		return nil, err
	}
	stats := map[string]float{
		"swap_total": float(swap.Total),
		"swap_used":  float(swap.Used),
		"swap_free":  float(swap.Free),
	}
	return stats, nil
}

func (h *HostClient) getHostFileSystemUsage() (map[string]float, error) {
	diskStat, err := disk.DiskUsage("/")
	if err != nil {
		return nil, err
	}
	stats := map[string]float{
		"disk_total": float(diskStat.Total),
		"disk_used":  float(diskStat.Used),
		"disk_free":  float(diskStat.Free),
	}
	return stats, nil
}

func (h *HostClient) getHostUptime() (map[string]float, error) {
	uptime, err := host.Uptime()
	if err != nil {
		return nil, err
	}
	stats := map[string]float{"uptime": float(uptime)}
	return stats, nil
}

func (h *HostClient) getHostCpuTimes() (map[string]float, error) {
	cpuStats, err := cpu.CPUTimes(false)
	if err != nil {
		return nil, err
	}
	stats := h.calculateCpuPercent(&cpuStats[0])
	h.lastCPUStats = &cpuStats[0]
	return stats, nil
}

func (h *HostClient) calculateCpuPercent(currentCpuStats *cpu.CPUTimesStat) map[string]float {
	var user, sys, idle, stolen, wait float64
	if h.lastCPUStats != nil {
		deltaTotal := currentCpuStats.Total() - h.lastCPUStats.Total()
		user = (currentCpuStats.User - h.lastCPUStats.User) / deltaTotal
		sys = (currentCpuStats.System - h.lastCPUStats.System) / deltaTotal
		idle = (currentCpuStats.Idle - h.lastCPUStats.Idle) / deltaTotal
		stolen = (currentCpuStats.Stolen - h.lastCPUStats.Stolen) / deltaTotal
		wait = (currentCpuStats.Iowait - h.lastCPUStats.Iowait) / deltaTotal
	}
	stats := map[string]float{
		"cpu_user":   float(user),
		"cpu_sys":    float(sys),
		"cpu_idle":   float(idle),
		"cpu_stolen": float(stolen),
		"cpu_wait":   float(wait),
		"cpu_busy":   float(user + sys),
	}
	return stats
}

func (h *HostClient) getHostNetworkUsage() (map[string]float, error) {
	netStat, err := net.NetIOCounters(true)
	if err != nil {
		return nil, err
	}
	for _, netInterface := range netStat {
		if netInterface.Name == h.ifaceName {
			stats := map[string]float{
				"netrx": float(netInterface.BytesRecv),
				"nettx": float(netInterface.BytesSent),
			}
			return stats, nil
		}
	}
	return nil, errInterfaceNotFound{name: h.ifaceName}
}

func (h *HostClient) GetHostname() (string, error) {
	hostInfo, err := host.HostInfo()
	if err != nil {
		return "", err
	}
	return hostInfo.Hostname, nil
}
