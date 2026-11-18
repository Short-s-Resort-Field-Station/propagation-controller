package api

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"propagation-controller/internal/version"
)

// Box health (2026-11-17): the things you want to know about the control box
// itself when the rack is fine but the link is not: which network it is on,
// how strong the signal is, which build it runs and since when. On the night
// the Wi-Fi adapter died these all had to be read over a cable.
//
// Every reading is best-effort and simply absent when the box cannot supply
// it (a simulator on a Mac has no /proc), so the screen shows what is known
// and never an invented value.

type boxNetwork struct {
	Interface     string `json:"interface"`
	Kind          string `json:"kind"` // "wifi" | "wired"
	Name          string `json:"name,omitempty"`
	SignalDBm     *int   `json:"signal_dbm,omitempty"`
	SignalPercent *int   `json:"signal_percent,omitempty"`
	Address       string `json:"address,omitempty"`
}

type boxView struct {
	Hostname      string      `json:"hostname,omitempty"`
	Build         string      `json:"build"`
	Version       string      `json:"version"`
	Simulation    bool        `json:"simulation"`
	Time          string      `json:"time"`
	DaemonUptime  int         `json:"daemon_uptime_seconds"`
	HostUptime    *int        `json:"host_uptime_seconds,omitempty"`
	InstalledAt   string      `json:"installed_at,omitempty"`
	Network       *boxNetwork `json:"network,omitempty"`
	DiskFreeBytes *uint64     `json:"disk_free_bytes,omitempty"`
	DiskSizeBytes *uint64     `json:"disk_size_bytes,omitempty"`
	CPUTempC      *float64    `json:"cpu_temp_c,omitempty"`
	Load1m        *float64    `json:"load_1m,omitempty"`
}

// handleBox implements GET /api/v1/system/box. Signed-in callers only: the
// network name and LAN address are nobody else's business, and unlike the
// rest of the read API this is served to the open internet through the tunnel.
func (s *Server) handleBox(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) == nil {
		writeError(w, http.StatusUnauthorized, "sign in to see the control box")
		return
	}
	v := boxView{
		Build:        version.Build,
		Version:      version.Version,
		Simulation:   s.simulation,
		Time:         time.Now().UTC().Format(time.RFC3339),
		DaemonUptime: int(time.Since(s.startedAt).Seconds()),
	}
	if h, err := os.Hostname(); err == nil {
		v.Hostname = h
	}
	if exe, err := os.Executable(); err == nil {
		if st, err := os.Stat(exe); err == nil {
			v.InstalledAt = st.ModTime().UTC().Format(time.RFC3339)
		}
		var fs syscall.Statfs_t
		if syscall.Statfs(filepath.Dir(exe), &fs) == nil {
			free := uint64(fs.Bavail) * uint64(fs.Bsize)
			size := uint64(fs.Blocks) * uint64(fs.Bsize)
			v.DiskFreeBytes, v.DiskSizeBytes = &free, &size
		}
	}
	if f := firstField("/proc/uptime"); f != "" {
		if up, err := strconv.ParseFloat(f, 64); err == nil {
			n := int(up)
			v.HostUptime = &n
		}
	}
	if f := firstField("/proc/loadavg"); f != "" {
		if l, err := strconv.ParseFloat(f, 64); err == nil {
			v.Load1m = &l
		}
	}
	v.CPUTempC = cpuTemp()
	v.Network = boxNet(r.Context())
	writeJSON(w, http.StatusOK, v)
}

func firstField(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// cpuTemp is the hottest thermal zone, in °C.
func cpuTemp() *float64 {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	var hot *float64
	for _, z := range zones {
		milli, err := strconv.ParseFloat(firstField(z), 64)
		if err != nil || milli <= 0 {
			continue
		}
		c := milli / 1000
		if hot == nil || c > *hot {
			hot = &c
		}
	}
	return hot
}

// boxNet describes the interface carrying the default route: the one the
// tunnel, and so the dashboard, depends on.
func boxNet(ctx context.Context) *boxNetwork {
	name := defaultRouteInterface()
	if name == "" {
		return nil
	}
	n := &boxNetwork{Interface: name, Kind: "wired"}
	if ifc, err := net.InterfaceByName(name); err == nil {
		if addrs, err := ifc.Addrs(); err == nil {
			for _, a := range addrs {
				if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
					n.Address = ipn.IP.String()
					break
				}
			}
		}
	}
	if _, err := os.Stat("/sys/class/net/" + name + "/wireless"); err == nil {
		n.Kind = "wifi"
		n.SignalDBm, n.SignalPercent = wirelessSignal(name)
	}
	// The NetworkManager connection name is the Wi-Fi network's name for a
	// Wi-Fi link. nmcli is optional; without it the name is simply omitted.
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(cctx, "nmcli", "-t", "-g", "GENERAL.CONNECTION", "device", "show", name).Output(); err == nil {
		n.Name = strings.TrimSpace(string(out))
	}
	return n
}

// defaultRouteInterface reads /proc/net/route for the lowest-metric default.
func defaultRouteInterface() string {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	best, bestMetric := "", int(^uint(0)>>1)
	for _, line := range strings.Split(string(b), "\n")[1:] {
		f := strings.Fields(line)
		if len(f) < 8 || f[1] != "00000000" {
			continue
		}
		metric, err := strconv.Atoi(f[6])
		if err != nil {
			continue
		}
		if metric < bestMetric {
			best, bestMetric = f[0], metric
		}
	}
	return best
}

// wirelessSignal reads /proc/net/wireless: link quality (out of 70) and level
// in dBm. Either may be missing; both nil when the interface is not listed.
func wirelessSignal(name string) (dbm, percent *int) {
	b, err := os.ReadFile("/proc/net/wireless")
	if err != nil {
		return nil, nil
	}
	return parseWireless(string(b), name)
}

func parseWireless(table, name string) (dbm, percent *int) {
	for _, line := range strings.Split(table, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, name+":") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 4 {
			return nil, nil
		}
		if q, err := strconv.ParseFloat(strings.TrimSuffix(f[2], "."), 64); err == nil {
			p := int(q/70*100 + 0.5)
			if p > 100 {
				p = 100
			}
			percent = &p
		}
		if l, err := strconv.ParseFloat(strings.TrimSuffix(f[3], "."), 64); err == nil && l < 0 {
			d := int(l)
			dbm = &d
		}
		return dbm, percent
	}
	return nil, nil
}
