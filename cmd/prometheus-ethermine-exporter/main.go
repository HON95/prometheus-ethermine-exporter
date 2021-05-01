package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"dev.hon.one/prometheus-ethermine-exporter/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "ethermine"

var poolAPIURLs = map[string]string{
	"ethermine":         "https://api.ethermine.org",
	"ethermine-etc":     "https://api-etc.ethermine.org",
	"ethpool":           "https://ethpool.org/api/pool",
	"flypool-zcash":     "https://api-zcash.flypool.org",
	"flypool-ravencoin": "https://api-ravencoin.flypool.org",
	"flypool-beam":      "https://api-beam.flypool.org",
}

const poolBasicAPIURLSuffix = "/poolStats"
const poolNetworkAPIURLSuffix = "/networkStats"
const poolServerAPIURLSuffix = "/servers/history"
const minerStatsAPIURLSuffixTemplate = "/miner/<miner>/currentStats"
const minerWorkersAPIURLSuffixTemplate = "/miner/<miner>/workers"

const defaultDebug = false
const defaultEndpoint = ":8080"

var enableDebug = false
var endpoint = defaultEndpoint

type baseAPIData struct {
	Status string `json:"status"`
}

type noDataAPIData struct {
	baseAPIData
	Data string `json:"data"`
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

type minerStatsAPIData struct {
	baseAPIData
	Data struct {
		Timestamp          float64 `json:"time"`
		LastSeenTimestamp  float64 `json:"lastSeen"`
		ReportedHashRate   float64 `json:"reportedHashrate"`
		CurrentHashRate    float64 `json:"currentHashrate"`
		AverageHashRate    float64 `json:"averageHashrate"`
		ValidShares        float64 `json:"validShares"`
		InvalidShares      float64 `json:"invalidShares"`
		StaleShares        float64 `json:"staleShares"`
		ActiveWorkers      float64 `json:"activeWorkers"`
		UnpaidBalance      float64 `json:"unpaid"`
		UnconfirmedBalance float64 `json:"unconfirmed"`
		CoinsPerMinute     float64 `json:"coinsPerMin"`
		BTCPerMinute       float64 `json:"btcPerMin"`
		USDPerMinute       float64 `json:"usdPerMin"`
	} `json:"data"`
}

type minerWorkersAPIData struct {
	baseAPIData
	Data []minerWorkersAPIDataElement `json:"data"`
}

type minerWorkersAPIDataElement struct {
	Name              string  `json:"worker"`
	Timestamp         float64 `json:"time"`
	LastSeenTimestamp float64 `json:"lastSeen"`
	ReportedHashRate  float64 `json:"reportedHashrate"`
	CurrentHashRate   float64 `json:"currentHashrate"`
	ValidShares       float64 `json:"validShares"`
	InvalidShares     float64 `json:"invalidShares"`
	StaleShares       float64 `json:"staleShares"`
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
	flag.BoolVar(&enableDebug, "debug", defaultDebug, "Show debug messages.")
	flag.StringVar(&endpoint, "endpoint", defaultEndpoint, "The address-port endpoint to bind to.")

	// Exits on error
	flag.Parse()
}

func runServer() error {
	fmt.Printf("Listening on %s.\n", endpoint)
	var mainServeMux http.ServeMux
	mainServeMux.HandleFunc("/", handleOtherRequest)
	mainServeMux.HandleFunc("/pool", handlePoolScrapeRequest)
	mainServeMux.HandleFunc("/miner", handleMinerScrapeRequest)
	if err := http.ListenAndServe(endpoint, &mainServeMux); err != nil {
		return fmt.Errorf("Error while running main HTTP server: %s", err)
	}
	return nil
}

func handleOtherRequest(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/" {
		fmt.Fprintf(response, "%s version %s by %s.\n", appName, appVersion, appAuthor)
		fmt.Fprintf(response, "\nPool IDs:\n")
		for _, poolID := range util.MapKeys(poolAPIURLs) {
			fmt.Fprintf(response, "- %s\n", poolID)
		}
		fmt.Fprintf(response, "\nMetrics paths:\n")
		fmt.Fprintf(response, "- Pool: /pool?pool=<pool>\n")
		fmt.Fprintf(response, "- Miner: /miner?pool=<pool>&target=<miner-address>\n")
	} else {
		message := fmt.Sprintf("404 - Page not found.\n")
		http.Error(response, message, 404)
	}
}

func handlePoolScrapeRequest(response http.ResponseWriter, request *http.Request) {
	if enableDebug {
		fmt.Printf("[DEBUG] Pool request: from=%s to=%v\n", request.RemoteAddr, request.URL.String())
	}

	// Get pool
	var poolID string
	if values, ok := request.URL.Query()["pool"]; ok && len(values) > 0 && values[0] != "" {
		poolID = values[0]
	} else {
		http.Error(response, "400 - Missing pool.\n", 400)
		return
	}
	poolURL, poolURLOK := poolAPIURLs[poolID]
	if !poolURLOK {
		http.Error(response, "400 - Invalid pool.\n", 400)
		return
	}

	// Scrape target and parse data
	var basicData poolBasicAPIData
	if !scrapeParse(&basicData, response, poolURL+poolBasicAPIURLSuffix) {
		return
	}
	var serverData poolServerAPIData
	if !scrapeParse(&serverData, response, poolURL+poolServerAPIURLSuffix) {
		return
	}

	// Build registry with data
	registry := buildPoolRegistry(response, &basicData, &serverData)
	if registry == nil {
		return
	}

	// Delegare final handling to Prometheus
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	handler.ServeHTTP(response, request)
}

func handleMinerScrapeRequest(response http.ResponseWriter, request *http.Request) {
	if enableDebug {
		fmt.Printf("[DEBUG] Miner request: from=%s to=%v\n", request.RemoteAddr, request.URL.String())
	}

	// Get pool
	var poolID string
	if values, ok := request.URL.Query()["pool"]; ok && len(values) > 0 && values[0] != "" {
		poolID = values[0]
	} else {
		http.Error(response, "400 - Missing pool.\n", 400)
		return
	}
	poolURL, poolURLOK := poolAPIURLs[poolID]
	if !poolURLOK {
		http.Error(response, "404 - Pool not found.\n", 404)
		return
	}

	// Get miner address
	var minerAddress string
	if values, ok := request.URL.Query()["target"]; ok && len(values) > 0 && values[0] != "" {
		minerAddress = values[0]
	} else {
		http.Error(response, "400 - Missing miner address.\n", 400)
		return
	}

	// Scrape target and parse data
	apiMinerStatsURL := strings.Replace(poolURL+minerStatsAPIURLSuffixTemplate, "<miner>", minerAddress, 1)
	var statsData minerStatsAPIData
	if !scrapeParse(&statsData, response, apiMinerStatsURL) {
		return
	}
	apiMinerWorkersURL := strings.Replace(poolURL+minerWorkersAPIURLSuffixTemplate, "<miner>", minerAddress, 1)
	var workersData minerWorkersAPIData
	if !scrapeParse(&workersData, response, apiMinerWorkersURL) {
		return
	}

	// Build registry with data
	registry := buildMinerRegistry(response, minerAddress, &statsData, &workersData)
	if registry == nil {
		return
	}

	// Delegare final handling to Prometheus
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	handler.ServeHTTP(response, request)
}

// Scrape the HTTP target, parse the result and check if the result is OK.
func scrapeParse(data interface{}, response http.ResponseWriter, targetURL string) bool {
	// Scrape
	rawData := util.ScrapeHTTPTarget(response, targetURL, enableDebug)

	// Check status
	var baseData baseAPIData
	if !util.ParseJSON(&baseData, response, rawData, false, enableDebug) {
		return false
	}
	if baseData.Status != "OK" {
		http.Error(response, "500 - API data not OK.\n", 500)
		return false
	}

	// Check if no data (ignore failed parse)
	var noDataData noDataAPIData
	if util.ParseJSON(&noDataData, response, rawData, true, enableDebug) {
		if noDataData.Data == "NO DATA" {
			http.Error(response, "404 - API data not found for pool.\n", 404)
			return false
		}
	}

	// Parse final data
	return util.ParseJSON(&data, response, rawData, false, enableDebug)
}

// Builds a new registry for the pool endpoint, adds scraped data to it and returns it if successful or nil if not.
func buildPoolRegistry(response http.ResponseWriter, basicData *poolBasicAPIData, serverData *poolServerAPIData) *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector())

	util.NewExporterMetric(registry, namespace, appVersion)

	// Basic stats
	util.NewGauge(registry, namespace, "pool", "hashrate_hps", "Current total hash rate of the pool (H/s).").Set(basicData.Data.Stats.HashRate)
	util.NewGauge(registry, namespace, "pool", "miner_count", "Current total number of miners in the pool.").Set(basicData.Data.Stats.MinerCount)
	util.NewGauge(registry, namespace, "pool", "worker_count", "Current total number of workers in the pool.").Set(basicData.Data.Stats.WorkerCount)
	util.NewGauge(registry, namespace, "pool", "price_usd", "Current price (USD).").Set(basicData.Data.Price.USD)
	util.NewGauge(registry, namespace, "pool", "price_btc", "Current price (BTC).").Set(basicData.Data.Price.BTC)

	// Server stats
	lastServerElements := make(map[string]*poolServerAPIDataElement)
	for _, element := range serverData.Data {
		existingElement, exists := lastServerElements[element.Server]
		if !exists || element.Time > existingElement.Time {
			var elementClone poolServerAPIDataElement
			elementClone = element
			lastServerElements[element.Server] = &elementClone
		}
	}
	serverLabels := make(prometheus.Labels)
	serverLabels["server"] = ""
	serverHashRateMetric := util.NewGaugeVec(registry, namespace, "pool", "server_hashrate_hps", "Current hash rate per server (H/s).", serverLabels)
	for server, element := range lastServerElements {
		labels := make(prometheus.Labels)
		labels["server"] = server
		serverHashRateMetric.With(labels).Set(element.HashRate)
	}

	return registry
}

// Builds a new registry for the miner endpoint, adds scraped data to it and returns it if successful or nil if not.
func buildMinerRegistry(response http.ResponseWriter, minerAddress string, statsData *minerStatsAPIData, workersData *minerWorkersAPIData) *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector())

	util.NewExporterMetric(registry, namespace, appVersion)

	// Note: Miner address isn't needed as it's the instance/target of the scrape.

	// Miner stats
	util.NewGauge(registry, namespace, "miner", "last_seen_seconds", "Delta between time of last statistics entry and when any workers from the miner was last seen (s).").Set(statsData.Data.Timestamp - statsData.Data.LastSeenTimestamp)
	util.NewGauge(registry, namespace, "miner", "hashrate_reported_hps", "Total hash rate for a miner as reported by the miner (H/s).").Set(statsData.Data.ReportedHashRate)
	util.NewGauge(registry, namespace, "miner", "hashrate_current_hps", "Total current hash rate for a miner (H/s).").Set(statsData.Data.CurrentHashRate)
	util.NewGauge(registry, namespace, "miner", "hashrate_average_hps", "Total average hash rate for a miner (H/s).").Set(statsData.Data.AverageHashRate)
	util.NewGauge(registry, namespace, "miner", "shares_valid", "Total number of valid shares for a miner.").Set(statsData.Data.ValidShares)
	util.NewGauge(registry, namespace, "miner", "shares_invalid", "Total number of invalid shares for a miner.").Set(statsData.Data.InvalidShares)
	util.NewGauge(registry, namespace, "miner", "shares_stale", "Total number of stale shares for a miner.").Set(statsData.Data.StaleShares)
	util.NewGauge(registry, namespace, "miner", "balance_unpaid_coins", "Unpaid balance for a miner (in the pool's native currency).").Set(statsData.Data.UnpaidBalance)
	util.NewGauge(registry, namespace, "miner", "balance_unconfirmed_coins", "Unconfirmed balance for a miner (in the pool's native currency).").Set(statsData.Data.UnconfirmedBalance)
	util.NewGauge(registry, namespace, "miner", "income_minute_coins", "Mined coins per minute (in the pool's native currency).").Set(statsData.Data.CoinsPerMinute)
	util.NewGauge(registry, namespace, "miner", "income_minute_usd", "Mined coins per minute (converted to USD).").Set(statsData.Data.USDPerMinute)
	util.NewGauge(registry, namespace, "miner", "income_minute_btc", "Mined coins per minute (converted to BTC).").Set(statsData.Data.BTCPerMinute)

	// Worker stats
	workerLabels := make(prometheus.Labels)
	workerLabels["worker"] = ""
	workerLastSeenMetric := util.NewGaugeVec(registry, namespace, "worker", "last_seen_seconds", "Delta between time of last statistics entry and when the miner was last seen (s).", workerLabels)
	workerReportedHashRateMetric := util.NewGaugeVec(registry, namespace, "worker", "hashrate_reported_hps", "Current hash rate for a worker as reported from the worker (H/s).", workerLabels)
	workerCurrentHashRateMetric := util.NewGaugeVec(registry, namespace, "worker", "hashrate_current_hps", "Current hash rate for a worker (H/s).", workerLabels)
	workerValidSharesMetric := util.NewGaugeVec(registry, namespace, "worker", "shares_valid", "Number of valid shared for a worker.", workerLabels)
	workerInvalidSharesMetric := util.NewGaugeVec(registry, namespace, "worker", "shares_invalid", "Number of invalid shared for a worker.", workerLabels)
	workerStaleSharesMetric := util.NewGaugeVec(registry, namespace, "worker", "shares_stale", "Number of stale shared for a worker.", workerLabels)
	for _, element := range workersData.Data {
		labels := make(prometheus.Labels)
		labels["worker"] = element.Name
		workerLastSeenMetric.With(labels).Set(element.Timestamp - element.LastSeenTimestamp)
		workerReportedHashRateMetric.With(labels).Set(element.ReportedHashRate)
		workerCurrentHashRateMetric.With(labels).Set(element.CurrentHashRate)
		workerValidSharesMetric.With(labels).Set(element.ValidShares)
		workerInvalidSharesMetric.With(labels).Set(element.InvalidShares)
		workerStaleSharesMetric.With(labels).Set(element.StaleShares)
	}

	return registry
}
