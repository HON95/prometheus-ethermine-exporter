package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

// ScrapeHTTPTarget - Scrapes the HTTP target and returns the data.
// Returns nil if not successful. Any errors are written to the response writer.
func ScrapeHTTPTarget(response http.ResponseWriter, targetURL string, debug bool) []byte {
	// Scrape
	if debug {
		fmt.Printf("[DEBUG] Sending scrape request: %s\n", targetURL)
	}
	scrapeRequest, scrapeRequestErr := http.NewRequest("GET", targetURL, nil)
	if scrapeRequestErr != nil {
		if debug {
			fmt.Printf("[DEBUG] Failed to make request to scrape target:\n%v\n", scrapeRequestErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", scrapeRequestErr)
		http.Error(response, message, 500)
		return nil
	}
	scrapeRequest.Header.Set("Accept", "application/json")
	scrapeClient := http.Client{}
	scrapeResponse, scrapeResponseErr := scrapeClient.Do(scrapeRequest)
	if scrapeResponseErr != nil {
		if debug {
			fmt.Printf("[DEBUG] Failed to scrape target:\n%v\n", scrapeResponseErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", scrapeResponseErr)
		http.Error(response, message, 500)
		return nil
	}
	defer scrapeResponse.Body.Close()
	rawData, rawDataErr := ioutil.ReadAll(scrapeResponse.Body)
	if rawDataErr != nil {
		if debug {
			fmt.Printf("[DEBUG] Failed to read data from target:\n%v\n", rawDataErr)
		}
		message := fmt.Sprintf("500 - Failed to scrape target: %s\n", rawDataErr)
		http.Error(response, message, 500)
		return nil
	}

	return rawData
}

// ParseJSON - Parses the data to JSON.
// Returns true if successful. Any errors are written to the response writer.
func ParseJSON(data interface{}, response http.ResponseWriter, rawData []byte, failSilently bool, debug bool) bool {
	if err := json.Unmarshal(rawData, &data); err != nil {
		if !failSilently {
			if debug {
				fmt.Printf("[DEBUG] Failed to unmarshal data from target:\n%v\n", err)
				fmt.Printf("[DEBUG] Raw data:\n%s\n", rawData)
			}
			message := fmt.Sprintf("500 - Failed to parse scraped data.\n")
			http.Error(response, message, 500)
		}
		return false
	}

	return true
}

// NewExporterMetric - Convenience function to create, register and set a gauge containing exporter info.
func NewExporterMetric(registry *prometheus.Registry, namespace string, version string) {
	infoLabels := make(prometheus.Labels)
	infoLabels["version"] = version
	NewGauge(registry, namespace, "exporter", "info", "Metadata about the exporter.", infoLabels).Set(1)
}

// NewGauge - Convenience function to create, register and return a gauge.
func NewGauge(registry *prometheus.Registry, namespace string, subsystem string, name string, help string, constLabels prometheus.Labels) prometheus.Gauge {
	var metric = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        help,
		ConstLabels: constLabels,
	})
	registry.MustRegister(metric)
	return metric
}

// NewGaugeVec - Convenience function to create, register and return a labeled gauge.
func NewGaugeVec(registry *prometheus.Registry, namespace string, subsystem string, name string, help string, constLabels prometheus.Labels, labels prometheus.Labels) *prometheus.GaugeVec {
	var metric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        help,
		ConstLabels: constLabels,
	}, MapKeys(labels))
	registry.MustRegister(metric)
	return metric
}

// MergeLabels - Merge multiple label maps into one. If they have overlapping keys, the value from the most right map will be used.
func MergeLabels(maps ...prometheus.Labels) prometheus.Labels {
	result := make(prometheus.Labels)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
