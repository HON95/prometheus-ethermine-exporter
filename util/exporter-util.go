package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

// ScrapeJSONTarget - Scrapes the HTTP target and parses the data.
// Returns true if successful. Any errors written to the response writer.
func ScrapeJSONTarget(response http.ResponseWriter, data interface{}, targetURL string, debug bool) bool {
	// Scrape
	scrapeRequest, scrapeRequestErr := http.NewRequest("GET", targetURL, nil)
	if scrapeRequestErr != nil {
		if debug {
			fmt.Printf("[DEBUG] Failed to make request to scrape target:\n%v\n", scrapeRequestErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", scrapeRequestErr)
		http.Error(response, message, 500)
		return false
	}
	scrapeClient := http.Client{}
	scrapeResponse, scrapeResponseErr := scrapeClient.Do(scrapeRequest)
	if scrapeResponseErr != nil {
		if debug {
			fmt.Printf("[DEBUG] Failed to scrape target:\n%v\n", scrapeResponseErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", scrapeResponseErr)
		http.Error(response, message, 500)
		return false
	}
	defer scrapeResponse.Body.Close()
	rawData, rawDataErr := ioutil.ReadAll(scrapeResponse.Body)
	if rawDataErr != nil {
		if debug {
			fmt.Printf("[DEBUG] Failed to read data from target:\n%v\n", rawDataErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", rawDataErr)
		http.Error(response, message, 500)
		return false
	}

	// Parse
	if err := json.Unmarshal(rawData, &data); err != nil {
		if debug {
			fmt.Printf("[DEBUG] Failed to unmarshal data from target:\n%v\n", err)
			fmt.Printf("[DEBUG] Raw data:\n%s\n", rawData)
		}
		message := fmt.Sprintf("500 - Failed to parse scraped data: %s\n", err)
		http.Error(response, message, 500)
		return false
	}

	return true
}

// NewExporterMetric - Convenience function to create, register and set a gauge containing exporter info.
func NewExporterMetric(registry *prometheus.Registry, namespace string, version string) {
	infoLabels := make(prometheus.Labels)
	infoLabels["version"] = version
	var infoMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "exporter_info",
		Help:      "Metadata about the exporter.",
	}, LabelsKeys(infoLabels))
	infoMetric.With(infoLabels).Set(1)
	registry.MustRegister(infoMetric)
}

// NewGauge - Convenience function to create, register and return a gauge.
func NewGauge(registry *prometheus.Registry, namespace string, subsystem string, name string, help string) prometheus.Gauge {
	var metric = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	})
	registry.MustRegister(metric)
	return metric
}

// NewGaugeVec - Convenience function to create, register and return a labeled gauge.
func NewGaugeVec(registry *prometheus.Registry, namespace string, subsystem string, name string, help string, labels prometheus.Labels) *prometheus.GaugeVec {
	var metric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      name,
		Help:      help,
	}, LabelsKeys(labels))
	registry.MustRegister(metric)
	return metric
}

// LabelsKeys - Extract the keys from the labels map.
func LabelsKeys(fullMap prometheus.Labels) []string {
	keys := make([]string, len(fullMap))
	i := 0
	for key := range fullMap {
		keys[i] = key
		i++
	}
	return keys
}
