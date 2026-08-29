//go:build darwin

package metrics

import (
	"encoding/binary"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

func ReadSystem() (System, error) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return System{}, err
	}
	release, err := unix.Sysctl("kern.osrelease")
	if err != nil {
		return System{}, err
	}
	version, _ := unix.Sysctl("kern.osversion")
	model, _ := unix.Sysctl("machdep.cpu.brand_string")
	if model == "" {
		model = runtime.GOARCH
	}
	freePages := darwinSysctlUint32("vm.page_free_count") + darwinSysctlUint32("vm.page_inactive_count")
	pageSize, _ := unix.SysctlUint64("hw.pagesize")
	boot := darwinBootTime()
	loads := [3]float64{}
	cpus := make([]CPUInfo, max(runtime.NumCPU(), 1))
	for index := range cpus {
		cpus[index] = CPUInfo{Model: model, Times: map[string]uint64{"user": 0, "nice": 0, "sys": 0, "idle": 0, "irq": 0}}
	}
	return System{
		Release: release, Version: version, Uptime: max(time.Since(boot).Seconds(), 0),
		LoadAvg: loads, TotalMem: total, FreeMem: uint64(freePages) * pageSize, CPUs: cpus,
	}, nil
}

func ReadProcess() (Process, error) {
	usage := unix.Rusage{}
	if err := unix.Getrusage(unix.RUSAGE_SELF, &usage); err != nil {
		return Process{}, err
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return Process{
		UserCPU:   time.Duration(usage.Utime.Sec)*time.Second + time.Duration(usage.Utime.Usec)*time.Microsecond,
		SystemCPU: time.Duration(usage.Stime.Sec)*time.Second + time.Duration(usage.Stime.Usec)*time.Microsecond,
		MaxRSS:    uint64(max(usage.Maxrss, 0)), RSS: memory.Sys,
		MinorPageFault: usage.Minflt, MajorPageFault: usage.Majflt, FSRead: usage.Inblock, FSWrite: usage.Oublock,
		VoluntarySwitches: usage.Nvcsw, InvoluntarySwitches: usage.Nivcsw,
	}, nil
}

func darwinSysctlUint32(name string) uint32 {
	raw, err := unix.SysctlRaw(name)
	if err != nil || len(raw) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(raw[:4])
}

func darwinBootTime() time.Time {
	raw, err := unix.SysctlRaw("kern.boottime")
	if err != nil || len(raw) < 8 {
		return ProcessStartedAt
	}
	seconds := int64(binary.LittleEndian.Uint64(raw[:8]))
	return time.Unix(seconds, 0)
}
