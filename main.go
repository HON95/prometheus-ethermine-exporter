package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const defaultEndpoint = ":8080"
const namespace = "ethermine"

const apiRootURL = "https://api.ethermine.org"
const apiPoolBasicURL = apiRootURL + "/poolStats"
const apiPoolNetworkURL = apiRootURL + "/networkStats"
const apiPoolServerURL = apiRootURL + "/servers/history"

var enableDebug = false
var endpoint = defaultEndpoint

type baseAPIData struct {
	Status string `json:"status"`
}

type poolAPIDataGroup struct {
	basicData  poolBasicAPIData
	serverData poolServerAPIData
}

type poolBasicAPIData struct {
	baseAPIData
	Data struct {
		Stats struct {
			HashRate    float64 `json:"hashRate"`
			MinerCount  float64 `json:"miners"`
			WorkerCount float64 `json:"workers"`
		} `json:"poolStats"`
		Price struct {
			USD float64 `json:"usd"`
			BTC float64 `json:"btc"`
		} `json:"price"`
	} `json:"data"`
}

type poolServerAPIData struct {
	baseAPIData
	Data []poolServerAPIDataElement `json:"data"`
}

type poolServerAPIDataElement struct {
	Time     int64   `json:"time"`
	HashRate float64 `json:"hashrate"`
	Server   string  `json:"server"`
}

func main() {
	fmt.Printf("%s version %s by %s.\n", appName, appVersion, appAuthor)

	parseCliArgs()
	if enableDebug {
		fmt.Printf("[DEBUG] Debug mode enabled.\n")
	}

	if err := runServer(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return
	}
}

func parseCliArgs() {
	flag.BoolVar(&enableDebug, "debug", false, "Show debug messages.")
	flag.StringVar(&endpoint, "endpoint", defaultEndpoint, "The address-port endpoint to bind to.")

	// Exits on error
	flag.Parse()
}

func runServer() error {
	fmt.Printf("Listening on %s.\n", endpoint)
	var mainServeMux http.ServeMux
	mainServeMux.HandleFunc("/", handleOtherRequest)
	mainServeMux.HandleFunc("/metrics/pool", handlePoolScrapeRequest)
	if err := http.ListenAndServe(endpoint, &mainServeMux); err != nil {
		return fmt.Errorf("Error while running main HTTP server: %s", err)
	}
	return nil
}

func handleOtherRequest(response http.ResponseWriter, request *http.Request) {
	fmt.Fprintf(response, "%s version %s by %s.\n\n", appName, appVersion, appAuthor)
	fmt.Fprintf(response, "Metric paths:\n")
	fmt.Fprintf(response, "- /metrics/pool\n")
}

func handlePoolScrapeRequest(response http.ResponseWriter, request *http.Request) {
	if enableDebug {
		fmt.Printf("[DEBUG] Request: %s\n", request.RemoteAddr)
	}

	// TODO reuse for miner address?
	// Get and parse target
	// targetURL := parseTargetURL(response, request)
	// if targetURL == nil {
	// 	return
	// }

	// Scrape target and parse data
	var data poolAPIDataGroup
	if !scrapeTarget(response, &data.basicData, apiPoolBasicURL) {
		return
	}
	if !scrapeTarget(response, &data.serverData, apiPoolServerURL) {
		return
	}

	// Build registry with data
	registry := buildPoolRegistry(response, &data)
	if registry == nil {
		return
	}

	// Delegare final handling to Prometheus
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	handler.ServeHTTP(response, request)
}

// Scrapes the target and returns the parsed data if successful or nil if not.
func scrapeTarget(response http.ResponseWriter, data interface{}, targetURL string) bool {
	// Scrape
	scrapeRequest, scrapeRequestErr := http.NewRequest("GET", targetURL, nil)
	if scrapeRequestErr != nil {
		if enableDebug {
			fmt.Printf("[DEBUG] Failed to make request to scrape target:\n%v\n", scrapeRequestErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", scrapeRequestErr)
		http.Error(response, message, 500)
		return false
	}
	scrapeClient := http.Client{}
	scrapeResponse, scrapeResponseErr := scrapeClient.Do(scrapeRequest)
	if scrapeResponseErr != nil {
		if enableDebug {
			fmt.Printf("[DEBUG] Failed to scrape target:\n%v\n", scrapeResponseErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", scrapeResponseErr)
		http.Error(response, message, 500)
		return false
	}
	defer scrapeResponse.Body.Close()
	rawData, rawDataErr := ioutil.ReadAll(scrapeResponse.Body)
	if rawDataErr != nil {
		if enableDebug {
			fmt.Printf("[DEBUG] Failed to read data from target:\n%v\n", rawDataErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", rawDataErr)
		http.Error(response, message, 500)
		return false
	}

	// Parse
	var baseData baseAPIData
	if err := json.Unmarshal(rawData, &baseData); err != nil {
		if enableDebug {
			fmt.Printf("[DEBUG] Failed to unmarshal data from target:\n%v\n", err)
			fmt.Printf("[DEBUG] Raw data:\n%s\n", rawData)
		}
		message := fmt.Sprintf("500 - Failed to parse scraped data: %s\n", err)
		http.Error(response, message, 500)
		return false
	}
	if baseData.Status != "OK" {
		if enableDebug {
			fmt.Printf("[DEBUG] Parsed received data is not OK.\n")
			fmt.Printf("[DEBUG] Raw data:\n%s\n", rawData)
		}
		message := fmt.Sprintf("500 - Received data is not OK.\n")
		http.Error(response, message, 500)
		return false
	}
	if err := json.Unmarshal(rawData, &data); err != nil {
		if enableDebug {
			fmt.Printf("[DEBUG] Failed to unmarshal data from target:\n%v\n", err)
			fmt.Printf("[DEBUG] Raw data:\n%s\n", rawData)
		}
		message := fmt.Sprintf("500 - Failed to parse scraped data: %s\n", err)
		http.Error(response, message, 500)
		return false
	}

	return true
}

// Builds a new registry, adds scraped data to it and returns it if successful or nil if not.
func buildPoolRegistry(response http.ResponseWriter, data *poolAPIDataGroup) *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector())

	addExporterMetrics(registry)

	// Basic stats
	newGauge(registry, "pool", "hashrate_hps", "Current total hashrate of the pool (H/s).").Set(data.basicData.Data.Stats.HashRate)
	newGauge(registry, "pool", "miner_count", "Current total number of miners in the pool.").Set(data.basicData.Data.Stats.MinerCount)
	newGauge(registry, "pool", "worker_count", "Current total number of workers in the pool.").Set(data.basicData.Data.Stats.WorkerCount)
	newGauge(registry, "pool", "price_usd", "Current price (USD).").Set(data.basicData.Data.Price.USD)
	newGauge(registry, "pool", "price_btc", "Current price (BTC).").Set(data.basicData.Data.Price.BTC)

	// Server stats
	lastServerElements := make(map[string]*poolServerAPIDataElement)
	for _, element := range data.serverData.Data {
		existingElement, exists := lastServerElements[element.Server]
		if !exists || element.Time > existingElement.Time {
			var elementClone poolServerAPIDataElement
			elementClone = element
			lastServerElements[element.Server] = &elementClone
		}
	}
	serverLabels := make(prometheus.Labels)
	serverLabels["server"] = ""
	serverHashRateMetric := newGaugeVec(registry, "pool", "server_hashrate_hps", "Current hashrate per server (H/s).", serverLabels)
	for server, element := range lastServerElements {
		labels := make(prometheus.Labels)
		labels["server"] = server
		serverHashRateMetric.With(labels).Set(element.HashRate)
	}

	return registry
}

func addExporterMetrics(registry *prometheus.Registry) {
	// Info
	infoLabels := make(prometheus.Labels)
	infoLabels["version"] = appVersion
	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "exporter_info",
		Help:      "Metadata about the exporter.",
	}, labelsKeys(infoLabels))
	infoMetric.With(infoLabels).Set(1)
	registry.MustRegister(infoMetric)
}

func newGauge(registry *prometheus.Registry, subsystem string, name string, help string) prometheus.Gauge {
	var metric = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})
	registry.MustRegister(metric)
	return metric
}

func newGaugeVec(registry *prometheus.Registry, subsystem string, name string, help string, labels prometheus.Labels) *prometheus.GaugeVec {
	var metric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	}, labelsKeys(labels))
	registry.MustRegister(metric)
	return metric
}

// func addSoftwareMetrics(registry *prometheus.Registry, data *apiData) {
// 	// Info
// 	infoLabels := make(prometheus.Labels)
// 	infoLabels["software"] = data.Software
// 	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "software_info",
// 		Help:      "Metadata about the software.",
// 	}, labelsKeys(infoLabels))
// 	infoMetric.With(infoLabels).Set(1)
// 	registry.MustRegister(infoMetric)
// }

// func addMiningMetrics(registry *prometheus.Registry, data *lolMinerMiningResult) {
// 	// Info
// 	infoLabels := make(prometheus.Labels)
// 	infoLabels["algorithm"] = data.Algorithm
// 	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "mining_info",
// 		Help:      "Metadata about mining.",
// 	}, labelsKeys(infoLabels))
// 	infoMetric.With(infoLabels).Set(1)
// 	registry.MustRegister(infoMetric)
// }

// func addStratumMetrics(registry *prometheus.Registry, data *lolMinerStratumResult) {
// 	// Common labels for subsystem
// 	commonLabels := make(prometheus.Labels)
// 	commonLabels["stratum_pool"] = data.CurrentPool
// 	commonLabels["stratum_user"] = data.CurrentUser

// 	// Info
// 	infoLabels := make(prometheus.Labels, len(commonLabels))
// 	for k, v := range commonLabels {
// 		infoLabels[k] = v
// 	}
// 	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "stratum_info",
// 		Help:      "Metadata about the stratum.",
// 	}, labelsKeys(infoLabels))
// 	infoMetric.With(infoLabels).Set(1)
// 	registry.MustRegister(infoMetric)

// 	// Avg. latency
// 	var avgLatencyMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "stratum_average_latency_seconds",
// 		Help:      "Average latency for the stratum (s).",
// 	}, labelsKeys(commonLabels))
// 	avgLatencyMetric.With(commonLabels).Set(data.AverageLatencyMs / 1000)
// 	registry.MustRegister(avgLatencyMetric)
// }

// func addSessionMetrics(registry *prometheus.Registry, data *lolMinerSessionResult) {
// 	// Common labels for subsystem
// 	commonLabels := make(prometheus.Labels)
// 	commonLabels["session_startup_time"] = data.StartupString

// 	// Info
// 	infoLabels := make(prometheus.Labels, len(commonLabels))
// 	for k, v := range commonLabels {
// 		infoLabels[k] = v
// 	}
// 	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "session_info",
// 		Help:      "Metadata about the session.",
// 	}, labelsKeys(infoLabels))
// 	infoMetric.With(infoLabels).Set(1)
// 	registry.MustRegister(infoMetric)

// 	// Startup
// 	var startupMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "session_startup_seconds_timestamp",
// 		Help:      "Timestamp for the start of the session.",
// 	}, labelsKeys(commonLabels))
// 	startupMetric.With(commonLabels).Set(float64(data.Startup))
// 	registry.MustRegister(startupMetric)

// 	// Uptime
// 	var uptimeMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "session_uptime_seconds",
// 		Help:      "Uptime for the session (s).",
// 	}, labelsKeys(commonLabels))
// 	uptimeMetric.With(commonLabels).Set(float64(data.Uptime))
// 	registry.MustRegister(uptimeMetric)

// 	// Last update
// 	var lastUpdateMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "session_last_update_seconds_timestamp",
// 		Help:      "Timestamp for last update.",
// 	}, labelsKeys(commonLabels))
// 	lastUpdateMetric.With(commonLabels).Set(float64(data.LastUpdate))
// 	registry.MustRegister(lastUpdateMetric)

// 	// Active GPUs
// 	var activeGPUsMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "session_active_gpus",
// 		Help:      "Number of active GPUs.",
// 	}, labelsKeys(commonLabels))
// 	activeGPUsMetric.With(commonLabels).Set(float64(data.ActiveGPUs))
// 	registry.MustRegister(activeGPUsMetric)

// 	// Performance
// 	var totalPerformanceMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "session_performance_total_mhps",
// 		Help:      "Total current performance for the session (Mh/s).",
// 	}, labelsKeys(commonLabels))
// 	totalPerformanceMetric.With(commonLabels).Set(float64(data.TotalPerformance))
// 	registry.MustRegister(totalPerformanceMetric)

// 	// Accepted shares
// 	var acceptedSharesMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
// 		Namespace: namespace,
// 		Name:      "session_accepted_shares_total",
// 		Help:      "Number of accepted shares for this session.",
// 	}, labelsKeys(commonLabels))
// 	acceptedSharesMetric.With(commonLabels).Add(float64(data.AcceptedShares))
// 	registry.MustRegister(acceptedSharesMetric)

// 	// Submitted shares
// 	var submittedSharesMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
// 		Namespace: namespace,
// 		Name:      "session_submitted_shares_total",
// 		Help:      "Number of submitted shares for this session.",
// 	}, labelsKeys(commonLabels))
// 	submittedSharesMetric.With(commonLabels).Add(float64(data.SubmittedShares))
// 	registry.MustRegister(submittedSharesMetric)

// 	// Total power
// 	var totalPowerMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "session_power_total_watts",
// 		Help:      "Total current power usage for the session (Watt).",
// 	}, labelsKeys(commonLabels))
// 	totalPowerMetric.With(commonLabels).Set(float64(data.TotalPower))
// 	registry.MustRegister(totalPowerMetric)
// }

// func addGPUMetrics(registry *prometheus.Registry, data *lolMinerGPUResult) {
// 	// Common labels for subsystem
// 	commonLabels := make(prometheus.Labels)
// 	commonLabels["gpu_index"] = fmt.Sprintf("%d", data.Index)

// 	// Info
// 	infoLabels := make(prometheus.Labels, len(commonLabels))
// 	for k, v := range commonLabels {
// 		infoLabels[k] = v
// 	}
// 	infoLabels["name"] = data.Name
// 	infoLabels["pcie_address"] = data.PCIEAddress
// 	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "gpu_info",
// 		Help:      "Metadata about a GPU.",
// 	}, labelsKeys(infoLabels))
// 	infoMetric.With(infoLabels).Set(1)
// 	registry.MustRegister(infoMetric)

// 	// Performance
// 	var performanceMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "gpu_performance_mhps",
// 		Help:      "GPU performance (Mh/s).",
// 	}, labelsKeys(commonLabels))
// 	performanceMetric.With(commonLabels).Set(float64(data.Performance))
// 	registry.MustRegister(performanceMetric)

// 	// Power
// 	var powerMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "gpu_power_watts",
// 		Help:      "GPU power usage (Watt).",
// 	}, labelsKeys(commonLabels))
// 	powerMetric.With(commonLabels).Set(float64(data.Power))
// 	registry.MustRegister(powerMetric)

// 	// Fan speed
// 	var fanSpeedMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "gpu_fan_speed",
// 		Help:      "GPU fan speed (0-1).",
// 	}, labelsKeys(commonLabels))
// 	fanSpeedMetric.With(commonLabels).Set(float64(data.FanSpeedPercent / 100))
// 	registry.MustRegister(fanSpeedMetric)

// 	// Temperature
// 	var temperatureMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
// 		Namespace: namespace,
// 		Name:      "gpu_temperature_celsius",
// 		Help:      "GPU temperature (deg. C).",
// 	}, labelsKeys(commonLabels))
// 	temperatureMetric.With(commonLabels).Set(float64(data.Temperature))
// 	registry.MustRegister(temperatureMetric)

// 	// Session accepted shares
// 	var sessionAcceptedSharesMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
// 		Namespace: namespace,
// 		Name:      "gpu_session_accepted_shares_total",
// 		Help:      "Number of accepted shared for the GPU during the current session.",
// 	}, labelsKeys(commonLabels))
// 	sessionAcceptedSharesMetric.With(commonLabels).Add(float64(data.SessionAcceptedShares))
// 	registry.MustRegister(sessionAcceptedSharesMetric)

// 	// Session submitted shares
// 	var sessionSubmittedSharesMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
// 		Namespace: namespace,
// 		Name:      "gpu_session_submitted_shares_total",
// 		Help:      "Number of submitted shared for the GPU during the current session.",
// 	}, labelsKeys(commonLabels))
// 	sessionSubmittedSharesMetric.With(commonLabels).Add(float64(data.SessionSubmittedShares))
// 	registry.MustRegister(sessionSubmittedSharesMetric)

// 	// Session HW errors
// 	var sessionHwErrorsMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
// 		Namespace: namespace,
// 		Name:      "gpu_session_hardware_errors_total",
// 		Help:      "Number of hardware errors for the GPU during the current session.",
// 	}, labelsKeys(commonLabels))
// 	sessionHwErrorsMetric.With(commonLabels).Add(float64(data.SessionHWErrors))
// 	registry.MustRegister(sessionHwErrorsMetric)
// }

func labelsKeys(fullMap prometheus.Labels) []string {
	keys := make([]string, len(fullMap))
	i := 0
	for key := range fullMap {
		keys[i] = key
		i++
	}
	return keys
}
