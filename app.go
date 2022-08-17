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
	req, err := http.NewRequest("GET", e.moonrakerEndpoint+printerInfo, nil)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 0,
		)
		log.Println(err)
		return
	}

	defer req.Body.Close()
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return
	}
	fmt.Println(string(body))

	ch <- prometheus.MustNewConstMetric(
		up, prometheus.GaugeValue, 1,
	)
}

func main() {
	moonrakerEndpoint := os.Getenv("MOONRAKER_ENDPOINT")

	exporter := NewExporter(moonrakerEndpoint)
	prometheus.MustRegister(exporter)

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9101", nil))
}
