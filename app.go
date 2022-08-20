package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	namespace = "moonraker"
)

var (
	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Was the last Moonraker query successful.",
		nil, nil,
	)
	// klipperStatus = prometheus.NewDesc(
	// 	prometheus.BuildFQName(namespace, "", "klipper_status"),
	// 	"Status of member in the wan cluster. 1=Alive, 2=Leaving, 3=Left, 4=Failed.",
	// 	[]string{"member", "dc"}, nil,
	// )
	extruderTemperature = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "temperature", "celsius"),
		"Temperature reading in degree Celsius.",
		[]string{"printer", "name"},
		nil,
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
	ch <- extruderTemperature
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ok := e.collectStatus(ch)
	ok = e.collectTemperature(ch) && ok
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
	fmt.Println("Collecting metrics from " + e.moonrakerEndpoint + "/printer/info")
	res, err := http.Get(e.moonrakerEndpoint + "/printer/info")
	if err != nil {
		fmt.Println("Failed to get Moonraker status")
		return false
	}
	defer res.Body.Close()
	body, error := ioutil.ReadAll(res.Body)
	if error != nil {
		fmt.Println(error)
		return false
	}
	fmt.Println(string(body))
	return true
}

type MoonrakerResponse struct {
	Result struct {
		Status struct {
			Extruder struct {
				PressureAdvance float64 `json:"pressure_advance"`
				Target          float64 `json:"target"`
				Power           float64 `json:"power"`
				CanExtrude      bool    `json:"can_extrude"`
				SmoothTime      float64 `json:"smooth_time"`
				Temperature     float64 `json:"temperature"`
			} `json:"extruder"`
		} `json:"status"`
	} `json:"result"`
}

func (e *Exporter) collectTemperature(ch chan<- prometheus.Metric) bool {
	fmt.Println("Collecting metrics from " + e.moonrakerEndpoint + "/printer/objects/query?extruder")
	res, err := http.Get(e.moonrakerEndpoint + "/printer/objects/query?extruder")
	if err != nil {
		fmt.Println("Failed to get temperature")
		return false
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return false
	}
	fmt.Println(string(data))

	var t MoonrakerResponse
	err = json.Unmarshal(data, &t)
	if err != nil {
		fmt.Println(err)
		return false
	}

	extruder := t.Result.Status.Extruder

	printerTag := os.Getenv("PRINTER_NAME")

	ch <- prometheus.MustNewConstMetric(
		extruderTemperature, prometheus.GaugeValue, extruder.Temperature, printerTag, "extruder",
	)
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
