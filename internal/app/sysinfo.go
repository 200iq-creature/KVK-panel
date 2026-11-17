package app

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type SysInfo struct {
	CPU       float64 `json:"cpu"`
	Cores     int     `json:"cores"`
	MemTotal  uint64  `json:"memTotal"`
	MemUsed   uint64  `json:"memUsed"`
	DiskTotal uint64  `json:"diskTotal"`
	DiskUsed  uint64  `json:"diskUsed"`
	Uptime    int64   `json:"uptime"`
	Load      string  `json:"load"`
	RxSpeed   float64 `json:"rxSpeed"` // bytes/s
	TxSpeed   float64 `json:"txSpeed"`
	RxTotal   uint64  `json:"rxTotal"`
	TxTotal   uint64  `json:"txTotal"`
	Iface     string  `json:"iface"`
	OS        string  `json:"os"`
	Arch      string  `json:"arch"`
}

type SysMonitor struct {
	mu   sync.Mutex
	info SysInfo
}

func newSysMonitor() *SysMonitor { return &SysMonitor{} }

func (s *SysMonitor) Get() SysInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info
}

func readCPU() (idle, total uint64) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	line := strings.SplitN(string(b), "\n", 2)[0]
	f := strings.Fields(line)
	for i, v := range f[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		total += n
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return
}

func readNet() (iface string, rx, tx uint64) {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return
	}
	var best uint64
	for _, line := range strings.Split(string(b), "\n")[2:] {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name == "lo" || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "br-") {
			continue
		}
		f := strings.Fields(parts[1])
		if len(f) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(f[0], 10, 64)
		t, _ := strconv.ParseUint(f[8], 10, 64)
		if r+t >= best {
			best = r + t
			iface, rx, tx = name, r, t
		}
	}
	return
}

func readMem() (total, used uint64) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	vals := map[string]uint64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			n, _ := strconv.ParseUint(f[1], 10, 64)
			vals[strings.TrimSuffix(f[0], ":")] = n * 1024
		}
	}
	total = vals["MemTotal"]
	avail := vals["MemAvailable"]
	if total > avail {
		used = total - avail
	}
	return
}

func (s *SysMonitor) loop(ctx context.Context) {
	pIdle, pTotal := readCPU()
	_, pRx, pTx := readNet()
	pT := time.Now()
	osName := "Linux"
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "PRETTY_NAME=") {
				osName = strings.Trim(strings.TrimPrefix(l, "PRETTY_NAME="), `"`)
			}
		}
	}
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		idle, total := readCPU()
		iface, rx, tx := readNet()
		now := time.Now()
		dt := now.Sub(pT).Seconds()
		info := SysInfo{Cores: runtime.NumCPU(), Iface: iface, RxTotal: rx, TxTotal: tx, OS: osName, Arch: runtime.GOARCH}
		if total > pTotal {
			info.CPU = 100 * (1 - float64(idle-pIdle)/float64(total-pTotal))
		}
		if dt > 0 && rx >= pRx && tx >= pTx {
			info.RxSpeed = float64(rx-pRx) / dt
			info.TxSpeed = float64(tx-pTx) / dt
		}
		info.MemTotal, info.MemUsed = readMem()
		var fs syscall.Statfs_t
		if syscall.Statfs("/", &fs) == nil {
			info.DiskTotal = fs.Blocks * uint64(fs.Bsize)
			info.DiskUsed = (fs.Blocks - fs.Bfree) * uint64(fs.Bsize)
		}
		if b, err := os.ReadFile("/proc/uptime"); err == nil {
			f := strings.Fields(string(b))
			if len(f) > 0 {
				u, _ := strconv.ParseFloat(f[0], 64)
				info.Uptime = int64(u)
			}
		}
		if b, err := os.ReadFile("/proc/loadavg"); err == nil {
			f := strings.Fields(string(b))
			if len(f) >= 3 {
				info.Load = f[0] + " " + f[1] + " " + f[2]
			}
		}
		s.mu.Lock()
		s.info = info
		s.mu.Unlock()
		pIdle, pTotal, pRx, pTx, pT = idle, total, rx, tx, now
	}
}
