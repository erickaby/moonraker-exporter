package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "printer_01"
const printerInfo = "/printer/info"

var (
	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Was the last Moonraker query successful.",
		nil, nil,
	)
)

type Exporter struct {
	moonrakerEndpoint string
}

func NewExporter(moonrakerEndpoint string) *Exporter {
	return &Exporter{
		moonrakerEndpoint: moonrakerEndpoint,
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ok := e.collectStatus(ch)
	// ok = e.collectLeaderMetric(ch) && ok
	if ok {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 1.0,
		)
	} else {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 0.0,
		)
	}
}

func (e *Exporter) collectStatus(ch chan<- prometheus.Metric) bool {
	fmt.Println("Collecting metrics from " + e.moonrakerEndpoint + printerInfo)
	req, err := http.NewRequest("GET", e.moonrakerEndpoint+printerInfo, nil)
	if err != nil {
		fmt.Println("Failed to get Moonraker status")
		return false
	}
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return false
	}
	fmt.Println(string(body))
	return true
}

func main() {
	moonrakerEndpoint := os.Getenv("MOONRAKER_ENDPOINT")

	fmt.Println("Starting server to collect Moonraker Api Metrics")
	fmt.Println("Preparing to collect metrics from " + moonrakerEndpoint)

	exporter := NewExporter(moonrakerEndpoint)
	prometheus.MustRegister(exporter)

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9101", nil))
}
