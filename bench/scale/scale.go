// Scale benchmark: generates synthetic configs at various object counts,
// starts gogios for each, measures check processing rate and LQL throughput.
//
// Usage: go run bench/scale/scale.go -binary ./gogios-bench -out bench/scale_results.csv
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func generateConfig(dir string, numHosts, svcsPerHost int, checkCmd string) error {
	os.MkdirAll(dir, 0755)
	os.MkdirAll(filepath.Join(dir, "var"), 0755)

	nagiosCfg := fmt.Sprintf(`log_file=var/nagios.log
cfg_file=commands.cfg
cfg_file=timeperiods.cfg
cfg_file=hosts.cfg
cfg_file=services.cfg
resource_file=resource.cfg
status_file=var/status.dat
status_update_interval=30
nagios_user=nagios
nagios_group=nagios
check_external_commands=0
interval_length=60
execute_service_checks=1
execute_host_checks=1
accept_passive_service_checks=1
accept_passive_host_checks=1
enable_notifications=0
enable_event_handlers=0
process_performance_data=0
check_for_updates=0
retain_state_information=0
enable_flap_detection=0
max_concurrent_checks=0
service_check_timeout=10
host_check_timeout=10
livestatus_tcp=127.0.0.1:6557
`)
	os.WriteFile(filepath.Join(dir, "nagios.cfg"), []byte(nagiosCfg), 0644)
	os.WriteFile(filepath.Join(dir, "resource.cfg"), []byte("$USER1$=/bin\n"), 0644)

	cmdsCfg := fmt.Sprintf(`define command {
    command_name    check_bench
    command_line    %s
}
`, checkCmd)
	os.WriteFile(filepath.Join(dir, "commands.cfg"), []byte(cmdsCfg), 0644)

	tpCfg := `define timeperiod {
    timeperiod_name  24x7
    alias            24x7
    sunday           00:00-24:00
    monday           00:00-24:00
    tuesday          00:00-24:00
    wednesday        00:00-24:00
    thursday         00:00-24:00
    friday           00:00-24:00
    saturday         00:00-24:00
}
`
	os.WriteFile(filepath.Join(dir, "timeperiods.cfg"), []byte(tpCfg), 0644)

	var hostsBuf strings.Builder
	for i := 0; i < numHosts; i++ {
		fmt.Fprintf(&hostsBuf, `define host {
    host_name       host-%05d
    alias           Bench Host %d
    address         10.%d.%d.%d
    check_command   check_bench
    max_check_attempts 3
    check_interval  0.1
    retry_interval  0.1
    check_period    24x7
    notification_period 24x7
}
`, i, i, (i/65536)%256, (i/256)%256, i%256)
	}
	os.WriteFile(filepath.Join(dir, "hosts.cfg"), []byte(hostsBuf.String()), 0644)

	var svcsBuf strings.Builder
	for i := 0; i < numHosts; i++ {
		for j := 0; j < svcsPerHost; j++ {
			fmt.Fprintf(&svcsBuf, `define service {
    host_name       host-%05d
    service_description svc-%03d
    check_command   check_bench
    max_check_attempts 3
    check_interval  0.1
    retry_interval  0.1
    check_period    24x7
    notification_period 24x7
}
`, i, j)
		}
	}
	os.WriteFile(filepath.Join(dir, "services.cfg"), []byte(svcsBuf.String()), 0644)

	return nil
}

func startGogios(binary, configDir string) (*exec.Cmd, error) {
	cfg := filepath.Join(configDir, "nagios.cfg")
	cmd := exec.Command(binary, cfg)
	cmd.Dir = configDir
	cmd.Stderr = cmd.Stdout // merge
	return cmd, nil
}

func killGogios(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
	// Also ensure nothing is left on the port
	time.Sleep(500 * time.Millisecond)
}

func waitForReady(cmd *exec.Cmd, timeout time.Duration) (time.Duration, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return 0, err
	}

	scanner := bufio.NewScanner(stdout)
	deadline := time.After(timeout)
	readyCh := make(chan struct{})

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Entering main event loop") || strings.Contains(line, "entering main event loop") || strings.Contains(line, "Gogios ready") {
				close(readyCh)
				for scanner.Scan() {
				}
				return
			}
		}
	}()

	select {
	case <-readyCh:
		return time.Since(start), nil
	case <-deadline:
		cmd.Process.Kill()
		return 0, fmt.Errorf("timeout waiting for gogios to start")
	}
}

func lqlQuery(addr, payload string) (time.Duration, bool) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return 0, false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	conn.Write([]byte(payload))

	buf := make([]byte, 1024*1024)
	n, _ := conn.Read(buf)
	return time.Since(start), n > 0
}

func benchLQL(addr, payload string, concurrency, iters int) (rps float64, p95ms float64) {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		lats []time.Duration
		oks  atomic.Int64
	)
	start := time.Now()
	for c := 0; c < concurrency; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				lat, ok := lqlQuery(addr, payload)
				if ok {
					oks.Add(1)
					mu.Lock()
					lats = append(lats, lat)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	wall := time.Since(start)
	rps = float64(oks.Load()) / wall.Seconds()

	if len(lats) > 0 {
		sortDurations(lats)
		idx := int(float64(len(lats)) * 0.95)
		if idx >= len(lats) {
			idx = len(lats) - 1
		}
		p95ms = float64(lats[idx].Microseconds()) / 1000.0
	}
	return
}

func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j] < d[j-1]; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}

func lqlQueryRaw(addr, payload string) string {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	conn.Write([]byte(payload))
	// Close write side so server sees EOF and closes after response
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.CloseWrite()
	}
	buf := make([]byte, 1024*1024)
	n, _ := conn.Read(buf)
	if n > 0 {
		return strings.TrimSpace(string(buf[:n]))
	}
	return ""
}

func measureCheckRate(addr string, totalServices int, dur time.Duration) float64 {
	// Use livestatus to count services checked in a time window.
	// Stats: last_check >= <unix_ts> returns count of services checked since ts.

	// Let checks stabilize
	time.Sleep(5 * time.Second)

	t1 := time.Now().Unix()
	time.Sleep(dur)
	q := fmt.Sprintf("GET services\nStats: last_check >= %d\n\n", t1)
	raw := lqlQueryRaw(addr, q)
	v, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if v > 0 {
		return float64(v) / dur.Seconds()
	}
	return 0
}

func getMemRSS(pid int) int64 {
	out, err := exec.Command("ps", "-o", "rss=", "-p", fmt.Sprintf("%d", pid)).Output()
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(out))
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func main() {
	binary := flag.String("binary", "./gogios-bench", "path to gogios binary")
	outFile := flag.String("out", "bench/scale_results.csv", "output CSV")
	checkCmd := flag.String("check", "/usr/bin/true", "check command to use (e.g. path to check_jitter binary)")
	onlyServices := flag.Int("only", 0, "run only the scenario with this many services (0=all)")
	flag.Parse()

	type scenario struct {
		hosts       int
		svcsPerHost int
	}
	allScenarios := []scenario{
		{10, 10},    // 100 services
		{50, 10},    // 500 services
		{100, 10},   // 1,000 services
		{200, 25},   // 5,000 services
		{500, 20},   // 10,000 services
		{1000, 50},  // 50,000 services
		{5000, 20},  // 100,000 services
		{10000, 20}, // 200,000 services
		{50000, 10}, // 500,000 services
	}
	var scenarios []scenario
	if *onlyServices > 0 {
		for _, sc := range allScenarios {
			if sc.hosts*sc.svcsPerHost == *onlyServices {
				scenarios = append(scenarios, sc)
			}
		}
		if len(scenarios) == 0 {
			fmt.Fprintf(os.Stderr, "No scenario with %d services found\n", *onlyServices)
			os.Exit(1)
		}
	} else {
		scenarios = allScenarios
	}

	f, err := os.Create(*outFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{
		"hosts", "services", "startup_ms", "mem_rss_kb",
		"checks_per_sec", "lql_hosts_rps", "lql_services_rps", "lql_stats_rps",
		"lql_hosts_p95_ms", "lql_services_p95_ms",
	})

	addr := "127.0.0.1:6557"

	for _, sc := range scenarios {
		totalSvcs := sc.hosts * sc.svcsPerHost
		fmt.Printf("\n=== %d hosts x %d svc/host = %d services ===\n", sc.hosts, sc.svcsPerHost, totalSvcs)

		configDir := filepath.Join(os.TempDir(), fmt.Sprintf("gogios-bench-%d", totalSvcs))
		os.RemoveAll(configDir)
		fmt.Printf("  Generating config in %s ...\n", configDir)
		generateConfig(configDir, sc.hosts, sc.svcsPerHost, *checkCmd)

		fmt.Printf("  Starting gogios ...\n")
		cmd, _ := startGogios(*binary, configDir)
		startupTime, err := waitForReady(cmd, 300*time.Second)
		if err != nil {
			fmt.Printf("  ERROR: %v, skipping\n", err)
			continue
		}
		fmt.Printf("  Started in %.1fms (PID %d)\n", float64(startupTime.Milliseconds()), cmd.Process.Pid)

		fmt.Printf("  Waiting for checks to start ...\n")
		time.Sleep(3 * time.Second)

		rssKB := getMemRSS(cmd.Process.Pid)
		fmt.Printf("  Memory RSS: %.1f MB\n", float64(rssKB)/1024)

		fmt.Printf("  Measuring check throughput (10s window) ...\n")
		checksPerSec := measureCheckRate(addr, totalSvcs, 10*time.Second)
		fmt.Printf("  Check throughput: %.0f checks/sec\n", checksPerSec)

		conc := 20
		iters := 50
		if totalSvcs >= 100000 {
			iters = 10
		} else if totalSvcs >= 50000 {
			iters = 20
		}

		fmt.Printf("  LQL benchmark (concurrency=%d, iters=%d) ...\n", conc, iters)

		hostsRPS, hostsP95 := benchLQL(addr, "GET hosts\nColumns: name state plugin_output\n\n", conc, iters)
		fmt.Printf("    hosts:    %6.0f rps  p95=%.1fms\n", hostsRPS, hostsP95)

		svcsRPS, svcsP95 := benchLQL(addr, "GET services\nColumns: host_name description state plugin_output\n\n", conc, iters)
		fmt.Printf("    services: %6.0f rps  p95=%.1fms\n", svcsRPS, svcsP95)

		statsRPS, _ := benchLQL(addr, "GET services\nStats: state = 0\nStats: state = 1\nStats: state = 2\nStats: state = 3\n\n", conc, iters)
		fmt.Printf("    stats:    %6.0f rps\n", statsRPS)

		killGogios(cmd)
		os.RemoveAll(configDir)

		w.Write([]string{
			fmt.Sprintf("%d", sc.hosts),
			fmt.Sprintf("%d", totalSvcs),
			fmt.Sprintf("%.1f", float64(startupTime.Milliseconds())),
			fmt.Sprintf("%d", rssKB),
			fmt.Sprintf("%.1f", checksPerSec),
			fmt.Sprintf("%.1f", hostsRPS),
			fmt.Sprintf("%.1f", svcsRPS),
			fmt.Sprintf("%.1f", statsRPS),
			fmt.Sprintf("%.3f", hostsP95),
			fmt.Sprintf("%.3f", svcsP95),
		})
		w.Flush()
	}
	fmt.Printf("\nResults written to %s\n", *outFile)
}
