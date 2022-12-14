package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
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
	pressureAdvanceSmoothTime = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "pressure_advance", "smooth_time"),
		"Pressure Advance in smooth time",
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
	ch <- pressureAdvance
	ch <- pressureAdvanceSmoothTime
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
	PressureAdvance float64 `mapstructure:"pressure_advance"`
	Target          float64 `mapstructure:"target"`
	Power           float64 `mapstructure:"power"`
	CanExtrude      bool    `mapstructure:"can_extrude"`
	SmoothTime      float64 `mapstructure:"smooth_time"`
	Temperature     float64 `mapstructure:"temperature"`
}

type HeaterBed struct {
	Target      float64 `mapstructure:"target"`
	Power       float64 `mapstructure:"power"`
	Temperature float64 `mapstructure:"temperature"`
}

type Fan struct {
	Speed float64 `mapstructure:"speed"`
}

type TemperatureFan struct {
	Speed       float64 `mapstructure:"speed"`
	Temperature float64 `mapstructure:"temperature"`
	Target      float64 `mapstructure:"target"`
}

type PrintStats struct {
	PrintDuration float64 `mapstructure:"print_duration"`
	TotalDuration float64 `mapstructure:"total_duration"`
	FilamentUsed  float64 `mapstructure:"filament_used"`
	Filename      string  `mapstructure:"filename"`
	State         string  `mapstructure:"state"`
	Message       string  `mapstructure:"message"`
}

type ObjectsConfig struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func getConfigObjectList() []ObjectsConfig {
	configPath := "./config/config.yaml"
	log.Debug("Reading config from file: " + configPath)
	configFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatal(err)
	}
	config := make(map[interface{}]interface{})

	log.Debug("Yaml Unmarshal yaml file")
	err = yaml.Unmarshal(configFile, &config)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	var objectList []ObjectsConfig
	log.Debug("mapstructure to objects")
	mapstructure.Decode(config["objects"], &objectList)

	return objectList
}

func (e *Exporter) collectObjectStatuses(ch chan<- prometheus.Metric) bool {
	log.Debug("Get list of objects from config")
	objectList := getConfigObjectList()
	log.Debug("Create query string from object list")
	strs := make([]string, len(objectList))
	for i, v := range objectList {
		log.Trace("Add to query: " + v.Name)
		query := url.QueryEscape(v.Name)
		log.Trace(query)
		strs[i] = query
	}
	log.Debug("Join list of query names")
	printerObjects := strings.Join(strs, "&")
	log.Debug(printerObjects)

	log.Info("Collecting metrics from " + e.moonrakerEndpoint + "/printer/objects/query?" + printerObjects)
	res, err := http.Get(e.moonrakerEndpoint + "/printer/objects/query?" + printerObjects)
	if err != nil {
		log.Warn("Failed to hit the object query endpoint")
		return false
	}
	defer res.Body.Close()
	log.Debug("Reading body of response")
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
		return false
	}
	log.Trace(data)

	var response MoonrakerObjectQueryResponse

	log.Debug("Json unmarshal body of response")
	err = json.Unmarshal(data, &response)
	if err != nil {
		log.Fatal(err)
		return false
	}

	log.Debug("Json unmarshaled:", response)

	printerTag := os.Getenv("PRINTER_NAME")
	for _, value := range objectList {
		object := response.Result.Status[value.Name]
		log.Trace("name: ", value.Name)
		log.Trace("type: ", value.Type)
		log.Trace("object: ", object)
		switch value.Type {
		case "Extruder":
			log.Trace("case = Extruder")
			var t Extruder
			mapstructure.Decode(object, &t)
			log.Trace(t.Temperature)
			ch <- prometheus.MustNewConstMetric(
				heaterTemperature, prometheus.GaugeValue, t.Temperature, printerTag, value.Name,
			)
			log.Trace(t.PressureAdvance)
			ch <- prometheus.MustNewConstMetric(
				pressureAdvance, prometheus.GaugeValue, t.PressureAdvance, printerTag, value.Name,
			)
			log.Trace(t.SmoothTime)
			ch <- prometheus.MustNewConstMetric(
				pressureAdvanceSmoothTime, prometheus.GaugeValue, t.SmoothTime, printerTag, value.Name,
			)
		case "Fan":
			log.Trace("case = Fan")
			var t Fan
			mapstructure.Decode(object, &t)
			log.Trace(t.Speed)
			ch <- prometheus.MustNewConstMetric(
				fanSpeed, prometheus.GaugeValue, t.Speed, printerTag, value.Name,
			)
		case "HeaterBed":
			log.Trace("case = HeaterBed")
			var t HeaterBed
			mapstructure.Decode(object, &t)
			log.Trace(t.Temperature)
			ch <- prometheus.MustNewConstMetric(
				heaterTemperature, prometheus.GaugeValue, t.Temperature, printerTag, value.Name,
			)
		case "TemperatureFan":
			log.Trace("case = TemperatureFan")
			var t TemperatureFan
			mapstructure.Decode(object, &t)
			log.Trace(t.Temperature)
			ch <- prometheus.MustNewConstMetric(
				heaterTemperature, prometheus.GaugeValue, t.Temperature, printerTag, value.Name,
			)
			log.Trace(t.Speed)
			ch <- prometheus.MustNewConstMetric(
				fanSpeed, prometheus.GaugeValue, t.Speed, printerTag, value.Name,
			)
		default:
			log.Trace("case = default")
			log.Warn("Not sure how to handle", value.Name, " with type ", value.Type)
		}
	}
	return true
}

func main() {
	logLevel := os.Getenv("LOG_LEVEL")
	switch logLevel {
	case "trace":
		log.SetLevel(log.TraceLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
	log.SetOutput(os.Stdout)

	moonrakerEndpoint := os.Getenv("MOONRAKER_ENDPOINT")

	log.Info("Starting server to collect Moonraker Api Metrics")
	log.Info("Preparing to collect metrics from " + moonrakerEndpoint)

	exporter := NewExporter(moonrakerEndpoint)
	prometheus.MustRegister(exporter)

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9101", nil))
}
