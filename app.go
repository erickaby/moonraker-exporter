package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	log "github.com/sirupsen/logrus"
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
	heaterTemperature = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "temperature", "celsius"),
		"Temperature reading in degree Celsius.",
		[]string{"printer", "name"},
		nil,
	)
	fanSpeed = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "fan", "percentage"),
		"Fan speed in percentage",
		[]string{"printer", "name"},
		nil,
	)
	pressureAdvance = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "pressure_advance", "amount"),
		"Pressure Advance in amount",
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
	ch <- heaterTemperature
	ch <- fanSpeed
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ok := e.collectStatus(ch)
	ok = e.collectObjectStatuses(ch) && ok
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
	log.Info("Collecting metrics from " + e.moonrakerEndpoint + "/printer/info")
	res, err := http.Get(e.moonrakerEndpoint + "/printer/info")
	if err != nil {
		log.Fatal("Failed to get Moonraker status")
		return false
	}
	defer res.Body.Close()
	body, error := ioutil.ReadAll(res.Body)
	if error != nil {
		log.Fatal(error)
		return false
	}
	log.Info(string(body))
	return true
}

type MoonrakerObjectQueryResponse struct {
	Result struct {
		Status map[string]interface{} `json:"status"`
	} `json:"result"`
}

type Extruder struct {
	PressureAdvance float64 `json:"pressure_advance"`
	Target          float64 `json:"target"`
	Power           float64 `json:"power"`
	CanExtrude      bool    `json:"can_extrude"`
	SmoothTime      float64 `json:"smooth_time"`
	Temperature     float64 `json:"temperature"`
}

type HeaterBed struct {
	Target      float64 `json:"target"`
	Power       float64 `json:"power"`
	Temperature float64 `json:"temperature"`
}

type Fan struct {
	Speed float64 `json:"speed"`
}

type TemperatureFan struct {
	Speed       float64 `json:"speed"`
	Temperature float64 `json:"temperature"`
	Target      float64 `json:"target"`
}

type PrintStats struct {
	PrintDuration float64 `json:"print_duration"`
	TotalDuration float64 `json:"total_duration"`
	FilamentUsed  float64 `json:"filament_used"`
	Filename      string  `json:"filename"`
	State         string  `json:"state"`
	Message       string  `json:"message"`
}

type ObjectsConfig struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func getConfigObjectList() []ObjectsConfig {
	configFile, err := ioutil.ReadFile("./config/config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	config := make(map[interface{}]interface{})

	yaml.Unmarshal(configFile, &config)

	var objectList []ObjectsConfig
	mapstructure.Decode(config["objects"], &objectList)

	return objectList
}

func (e *Exporter) collectObjectStatuses(ch chan<- prometheus.Metric) bool {
	objectList := getConfigObjectList()
	strs := make([]string, len(objectList))
	for i, v := range objectList {
		strs[i] = v.Name
	}
	printerObjects := strings.Join(strs, "&")
	log.Info("Collecting metrics from " + e.moonrakerEndpoint + "/printer/objects/query?" + printerObjects)
	res, err := http.Get(e.moonrakerEndpoint + "/printer/objects/query?" + printerObjects)
	if err != nil {
		log.Warn("Failed to hit the object query endpoint")
		return false
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
		return false
	}

	var response MoonrakerObjectQueryResponse

	err = json.Unmarshal(data, &response)
	if err != nil {
		log.Fatal(err)
		return false
	}

	log.Info("moonraker response", response)

	printerTag := os.Getenv("PRINTER_NAME")
	for _, value := range objectList {
		object := response.Result.Status[value.Name]
		log.Info("name: ", value.Name)
		log.Info("type: ", value.Type)
		switch value.Type {
		case "Extruder":
			var t Extruder
			mapstructure.Decode(object, &t)
			ch <- prometheus.MustNewConstMetric(
				heaterTemperature, prometheus.GaugeValue, t.Temperature, printerTag, value.Name,
			)
			ch <- prometheus.MustNewConstMetric(
				pressureAdvance, prometheus.GaugeValue, t.PressureAdvance, printerTag, value.Name,
			)
		case "Fan":
			var t Fan
			mapstructure.Decode(object, &t)
			ch <- prometheus.MustNewConstMetric(
				fanSpeed, prometheus.GaugeValue, t.Speed, printerTag, value.Name,
			)
		case "HeaterBed":
			var t HeaterBed
			mapstructure.Decode(object, &t)
			ch <- prometheus.MustNewConstMetric(
				heaterTemperature, prometheus.GaugeValue, t.Temperature, printerTag, value.Name,
			)
		case "TemperatureFan":
			var t TemperatureFan
			mapstructure.Decode(object, &t)
			ch <- prometheus.MustNewConstMetric(
				heaterTemperature, prometheus.GaugeValue, t.Temperature, printerTag, value.Name,
			)
			ch <- prometheus.MustNewConstMetric(
				fanSpeed, prometheus.GaugeValue, t.Speed, printerTag, value.Name,
			)
		default:
			log.Warn("Not sure how to handle", value.Name, " with type ", value.Type)
		}
	}
	return true
}

func main() {
	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	moonrakerEndpoint := os.Getenv("MOONRAKER_ENDPOINT")

	log.Info("Starting server to collect Moonraker Api Metrics")
	log.Info("Preparing to collect metrics from " + moonrakerEndpoint)

	exporter := NewExporter(moonrakerEndpoint)
	prometheus.MustRegister(exporter)

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9101", nil))
}
