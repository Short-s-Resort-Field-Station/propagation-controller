// propagation-watchdog — the independent supervisor (spec §4, §12 rule 10).
//
// It polls propagationd's health endpoint. On sustained failure it restarts
// the service; if restarts don't bring it back, it stops the service, takes
// the relay serial port (which the dead service no longer holds), and forces
// the safe state: pump and valves off. It never opens the port while
// propagationd is meant to be running.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"propagation-controller/internal/devices/relay"
	"propagation-controller/internal/watchdog"
)

func main() {
	healthURL := flag.String("health-url", "http://localhost/api/v1/health", "propagationd health endpoint")
	interval := flag.Duration("interval", 10*time.Second, "health check interval")
	failThreshold := flag.Int("fail-threshold", 3, "consecutive failures before restarting")
	restartLimit := flag.Int("restart-limit", 2, "restarts without recovery before forcing safe relay state")
	restartCmd := flag.String("restart-cmd", "systemctl restart propagationd", "command to restart the service")
	stopCmd := flag.String("stop-cmd", "systemctl stop propagationd", "command to stop the service before seizing the relay port")
	relayPort := flag.String("relay-port", "", "Numato serial port for forced safe state (autodetected if empty)")
	flag.Parse()

	logf("watchdog starting: url=%s interval=%s threshold=%d restart-limit=%d",
		*healthURL, *interval, *failThreshold, *restartLimit)

	d := watchdog.NewDecider(*failThreshold, *restartLimit)
	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for range ticker.C {
		healthy := checkHealth(client, *healthURL)
		switch d.Step(healthy) {
		case watchdog.ActionRestart:
			logf("health failing — restarting propagationd")
			runCmd(*restartCmd)
		case watchdog.ActionForceSafe:
			logf("restarts exhausted — stopping service and forcing safe relay state")
			runCmd(*stopCmd)
			forceSafeState(*relayPort)
			logf("safe state forced; resuming restart attempts")
			runCmd(*restartCmd)
		}
	}
}

// checkHealth: alive means the endpoint answers 200 with database "ok".
// A degraded relay/modbus still counts as alive — the service itself is up
// and surfacing the problem; restarting would not help.
func checkHealth(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		logf("health check failed: %v", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		logf("health returned HTTP %d", resp.StatusCode)
		return false
	}
	var body struct {
		Database string `json:"database"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		logf("health response unreadable: %v", err)
		return false
	}
	if body.Database != "ok" {
		logf("database reports %q", body.Database)
		return false
	}
	return true
}

// forceSafeState opens the Numato directly and turns off the pump, valves,
// and fans (channels 2–7). Lights are left alone — a dead controller is no
// reason to dark the plants.
func forceSafeState(port string) {
	if port == "" {
		detected, err := relay.DetectNumatoPort()
		if err != nil {
			logf("cannot force safe state: %v", err)
			return
		}
		port = detected
	}
	ctrl := relay.NewNumatoUSB(port)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := ctrl.Open(ctx); err != nil {
		logf("cannot open relay port %s: %v", port, err)
		return
	}
	defer ctrl.Close()
	for ch := 2; ch <= 7; ch++ {
		if err := ctrl.SetChannel(ctx, ch, relay.Off); err != nil {
			logf("relay %d off failed: %v", ch, err)
		} else {
			logf("relay %d forced off", ch)
		}
	}
}

func runCmd(cmd string) {
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		logf("command %q failed: %v (%s)", cmd, err, out)
	}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, time.Now().UTC().Format(time.RFC3339)+" watchdog: "+format+"\n", args...)
}
