// NRDP passive check benchmark: submits batches of passive check results via
// HTTP POST to a gogios NRDP endpoint, measures ingestion throughput, latency,
// and dynamic host/service registration overhead.
//
// Usage: go run bench/nrdp/bench.go -binary ./gogios-bench -out bench/nrdp_results.csv
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// generateConfig creates a minimal gogios config with NRDP enabled.
// No hosts or services are pre-configured — everything is dynamically created.
func generateConfig(dir, nrdpAddr string) error {
	os.MkdirAll(filepath.Join(dir, "var"), 0755)

	nagiosCfg := fmt.Sprintf(`log_file=var/nagios.log
cfg_file=objects.cfg
resource_file=resource.cfg
status_file=var/status.dat
status_update_interval=60
nagios_user=nagios
nagios_group=nagios
check_external_commands=0
interval_length=60
execute_service_checks=0
execute_host_checks=0
accept_passive_service_checks=1
accept_passive_host_checks=1
enable_notifications=0
enable_event_handlers=0
process_performance_data=0
retain_state_information=0
enable_flap_detection=0
max_concurrent_checks=0
livestatus_tcp=127.0.0.1:6557
nrdp_listen=%s
nrdp_path=/nrdp/
nrdp_dynamic_enabled=1
nrdp_dynamic_ttl=86400
nrdp_dynamic_prune_interval=3600
`, nrdpAddr)
	os.WriteFile(filepath.Join(dir, "nagios.cfg"), []byte(nagiosCfg), 0644)
	os.WriteFile(filepath.Join(dir, "resource.cfg"), []byte("$USER1$=/bin\n"), 0644)

	// Minimal objects: just a timeperiod so config validates
	objectsCfg := `define timeperiod {
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
define command {
    command_name    check_dummy
    command_line    /usr/bin/true
}
define contact {
    contact_name    admin
    host_notifications_enabled 0
    service_notifications_enabled 0
    host_notification_period 24x7
    service_notification_period 24x7
    host_notification_options d,u,r
    service_notification_options w,u,c,r
    host_notification_commands check_dummy
    service_notification_commands check_dummy
}
`
	os.WriteFile(filepath.Join(dir, "objects.cfg"), []byte(objectsCfg), 0644)
	return nil
}

func startGogios(binary, configDir string) (*exec.Cmd, error) {
	cfg := filepath.Join(configDir, "nagios.cfg")
	cmd := exec.Command(binary, cfg)
	cmd.Dir = configDir
	cmd.Stderr = cmd.Stdout
	return cmd, nil
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
			if strings.Contains(scanner.Text(), "Gogios ready") || strings.Contains(scanner.Text(), "Entering main event loop") {
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

func killGogios(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}
	time.Sleep(500 * time.Millisecond)
}

func getMemRSS(pid int) int64 {
	out, err := exec.Command("ps", "-o", "rss=", "-p", fmt.Sprintf("%d", pid)).Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return v
}

// buildXMLPayload creates an NRDP XML form body with `batchSize` check results.
// Each result has a unique hostname and service name within the batch.
func buildXMLPayload(hostOffset, batchSize int) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?><checkresults>`)
	for i := 0; i < batchSize; i++ {
		hostIdx := hostOffset + (i / 10)
		svcIdx := i % 10
		fmt.Fprintf(&sb, `<checkresult type="service" checktype="1">`+
			`<hostname>nrdp-host-%06d</hostname>`+
			`<servicename>svc-%03d</servicename>`+
			`<state>0</state>`+
			`<output>OK - bench result %d | rtt=0.001s;1;5;0</output>`+
			`</checkresult>`, hostIdx, svcIdx, i)
	}
	sb.WriteString(`</checkresults>`)
	return sb.String()
}

// submitBatch sends a single NRDP POST with batchSize results.
func submitBatch(nrdpURL string, hostOffset, batchSize int) (time.Duration, int, error) {
	xml := buildXMLPayload(hostOffset, batchSize)
	form := url.Values{
		"cmd":     {"submitcheck"},
		"XMLDATA": {xml},
	}
	start := time.Now()
	resp, err := http.PostForm(nrdpURL, form)
	lat := time.Since(start)
	if err != nil {
		return lat, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return lat, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return lat, batchSize, nil
}

// benchNRDP runs a sustained load test, sending batches of passive checks
// from multiple concurrent goroutines. Returns results/sec and P95 latency.
func benchNRDP(nrdpURL string, totalResults, batchSize, concurrency int) (rps float64, p95ms float64, totalSent int) {
	batches := totalResults / batchSize
	if batches < 1 {
		batches = 1
	}
	batchesPerWorker := batches / concurrency
	if batchesPerWorker < 1 {
		batchesPerWorker = 1
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		lats []time.Duration
		sent atomic.Int64
	)

	start := time.Now()
	for c := 0; c < concurrency; c++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < batchesPerWorker; i++ {
				hostOffset := (workerID*batchesPerWorker + i) * (batchSize / 10)
				lat, n, err := submitBatch(nrdpURL, hostOffset, batchSize)
				if err == nil {
					sent.Add(int64(n))
					mu.Lock()
					lats = append(lats, lat)
					mu.Unlock()
				}
			}
		}(c)
	}
	wg.Wait()
	wall := time.Since(start)

	totalSent = int(sent.Load())
	rps = float64(totalSent) / wall.Seconds()

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

func main() {
	binary := flag.String("binary", "./gogios-bench", "path to gogios binary")
	outFile := flag.String("out", "bench/nrdp_results.csv", "output CSV")
	nrdpPort := flag.String("port", "15669", "NRDP listen port")
	onlyResults := flag.Int("only", 0, "run only the scenario with this target count (0=all)")
	flag.Parse()

	nrdpAddr := "127.0.0.1:" + *nrdpPort
	nrdpURL := "http://" + nrdpAddr + "/nrdp/"

	type scenario struct {
		label       string
		totalChecks int // total passive results to submit
		batchSize   int // results per HTTP POST
		concurrency int // parallel HTTP clients
	}

	allScenarios := []scenario{
		{"100", 1000, 10, 1},        // 100 unique services, 10 batches
		{"500", 5000, 50, 2},        // 500 unique services
		{"1000", 10000, 100, 4},     // 1k unique services
		{"5000", 50000, 100, 8},     // 5k unique services
		{"10000", 100000, 100, 10},  // 10k unique services
		{"50000", 200000, 500, 20},  // 50k unique services
		{"100000", 400000, 500, 20}, // 100k unique services
	}

	var scenarios []scenario
	if *onlyResults > 0 {
		for _, sc := range allScenarios {
			v, _ := strconv.Atoi(sc.label)
			if v == *onlyResults {
				scenarios = append(scenarios, sc)
			}
		}
		if len(scenarios) == 0 {
			fmt.Fprintf(os.Stderr, "No scenario matching %d found\n", *onlyResults)
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
		"unique_services", "total_submitted", "batch_size", "concurrency",
		"results_per_sec", "p95_batch_ms", "mem_rss_kb",
	})

	for _, sc := range scenarios {
		fmt.Printf("\n=== %s unique services (submit %d results, batch=%d, conc=%d) ===\n",
			sc.label, sc.totalChecks, sc.batchSize, sc.concurrency)

		configDir := filepath.Join(os.TempDir(), fmt.Sprintf("gogios-nrdp-bench-%s", sc.label))
		os.RemoveAll(configDir)
		fmt.Printf("  Generating config in %s ...\n", configDir)
		generateConfig(configDir, nrdpAddr)

		fmt.Printf("  Starting gogios ...\n")
		cmd, _ := startGogios(*binary, configDir)
		startupTime, err := waitForReady(cmd, 60*time.Second)
		if err != nil {
			fmt.Printf("  ERROR: %v, skipping\n", err)
			continue
		}
		fmt.Printf("  Started in %.1fms (PID %d)\n", float64(startupTime.Milliseconds()), cmd.Process.Pid)

		// Let NRDP listener bind
		time.Sleep(1 * time.Second)

		// Warm up: send a small batch to create initial connections
		submitBatch(nrdpURL, 999999, 10)
		time.Sleep(500 * time.Millisecond)

		fmt.Printf("  Running NRDP load test ...\n")
		rps, p95, totalSent := benchNRDP(nrdpURL, sc.totalChecks, sc.batchSize, sc.concurrency)
		fmt.Printf("  Results: %.0f results/sec, P95 batch latency: %.1fms, total sent: %d\n", rps, p95, totalSent)

		// Measure memory after ingestion
		time.Sleep(1 * time.Second)
		rssKB := getMemRSS(cmd.Process.Pid)
		fmt.Printf("  Memory RSS: %.1f MB\n", float64(rssKB)/1024)

		killGogios(cmd)
		os.RemoveAll(configDir)

		w.Write([]string{
			sc.label,
			fmt.Sprintf("%d", totalSent),
			fmt.Sprintf("%d", sc.batchSize),
			fmt.Sprintf("%d", sc.concurrency),
			fmt.Sprintf("%.1f", rps),
			fmt.Sprintf("%.3f", p95),
			fmt.Sprintf("%d", rssKB),
		})
		w.Flush()
	}
	fmt.Printf("\nResults written to %s\n", *outFile)
}
