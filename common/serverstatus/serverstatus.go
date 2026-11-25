// Package serverstatus generate the server system status
package serverstatus

import (
	"fmt"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

// Status is the host's resource usage in absolute figures (bytes, seconds).
// Fields that could not be read stay zero.
type Status struct {
	CPU       float64 // percent
	MemTotal  uint64
	MemUsed   uint64
	SwapTotal uint64
	SwapUsed  uint64
	DiskTotal uint64
	DiskUsed  uint64
	Uptime    uint64
}

// GetSystemStatus reads the host's resource usage. On a partial failure it
// returns what it could read together with an error describing the rest.
func GetSystemStatus() (Status, error) {
	var status Status
	var problems []string

	if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
		status.CPU = cpuPercent[0]
	} else {
		problems = append(problems, fmt.Sprintf("cpu: %v", err))
	}
	if memory, err := mem.VirtualMemory(); err == nil {
		status.MemTotal, status.MemUsed = memory.Total, memory.Used
	} else {
		problems = append(problems, fmt.Sprintf("memory: %v", err))
	}
	if swap, err := mem.SwapMemory(); err == nil {
		status.SwapTotal, status.SwapUsed = swap.Total, swap.Used
	} else {
		problems = append(problems, fmt.Sprintf("swap: %v", err))
	}
	if usage, err := disk.Usage("/"); err == nil {
		status.DiskTotal, status.DiskUsed = usage.Total, usage.Used
	} else {
		problems = append(problems, fmt.Sprintf("disk: %v", err))
	}
	if uptime, err := host.Uptime(); err == nil {
		status.Uptime = uptime
	} else {
		problems = append(problems, fmt.Sprintf("uptime: %v", err))
	}

	if len(problems) > 0 {
		return status, fmt.Errorf("incomplete system status: %s", strings.Join(problems, "; "))
	}
	return status, nil
}

// GetSystemInfo get the system info of a given periodic
func GetSystemInfo() (Cpu float64, Mem float64, Disk float64, Uptime uint64, err error) {

	errorString := ""

	cpuPercent, err := cpu.Percent(0, false)
	// Check if cpuPercent is empty
	if len(cpuPercent) > 0 && err == nil {
		Cpu = cpuPercent[0]
	} else {
		Cpu = 0
		errorString += fmt.Sprintf("get cpu usage failed: %s ", err)
	}

	memUsage, err := mem.VirtualMemory()
	if err != nil {
		errorString += fmt.Sprintf("get mem usage failed: %s ", err)
	} else {
		Mem = memUsage.UsedPercent
	}

	diskUsage, err := disk.Usage("/")
	if err != nil {
		errorString += fmt.Sprintf("get disk usage failed: %s ", err)
	} else {
		Disk = diskUsage.UsedPercent
	}

	uptime, err := host.Uptime()
	if err != nil {
		errorString += fmt.Sprintf("get uptime failed: %s ", err)
	} else {
		Uptime = uptime
	}

	if errorString != "" {
		err = fmt.Errorf("%s", errorString)
	}

	return Cpu, Mem, Disk, Uptime, err
}
