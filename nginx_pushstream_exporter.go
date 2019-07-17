package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tkliner/nginx_push_stream_exporter/pushstream"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
)

const (
	namespace = "nginx" // For Prometheus metrics.
)

var (
	nginxLabelNames = []string{"channel"}
)

func newPushStreamMetrics(metricName string, docString string, constLabels prometheus.Labels) *prometheus.Desc {
	return prometheus.NewDesc(prometheus.BuildFQName(namespace, "push_stream", metricName), docString, nginxLabelNames, constLabels)
}

// Basic map of metrics types
type metrics map[string]*prometheus.Desc

// Convert metrics type map to string separed by comma
func (m metrics) String() string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s := make([]string, len(keys))
	for i, k := range keys {
		s[i] = k
	}
	return strings.Join(s, ",")
}

var (
	pushStreamMetrics = metrics{
		"channels":           newPushStreamMetrics("channels", "Current number of existing channels on this server.", nil),
		"subscribers":        newPushStreamMetrics("subscribers", "Current number of connected subscribers on channels on this server.", nil),
		"published_messages": newPushStreamMetrics("published_messages", "Current number of existing channels on this server.", nil),
		"stored_messages":    newPushStreamMetrics("stored_messages", "Current number of existing channels on this server.", nil),
		"subscribers_total":  newPushStreamMetrics("subscribers_total", "Total current number of connected subscribers on this server.", nil),
	}

	nginxUp = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "up"), "Was the last scrape of nginx successful.", nil, nil)
)

// Exporter collects Nginx pushStream stats from the given URI and exports them using
// the prometheus metrics package.
type Exporter struct {
	URI   string
	mutex sync.RWMutex
	fetch func() (io.ReadCloser, error)

	up                prometheus.Gauge
	totalScrapes      prometheus.Counter
	pushStreamMetrics map[string]*prometheus.Desc
}

// NewExporter returns an initialized Exporter.
func NewExporter(uri string, selectedPushStreamMetrics map[string]*prometheus.Desc, timeout time.Duration) (*Exporter, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	var fetch func() (io.ReadCloser, error)
	switch u.Scheme {
	case "http", "https", "file":
		fetch = fetchHTTP(uri, timeout)
	default:
		return nil, fmt.Errorf("unsupported scheme: %q", u.Scheme)
	}

	return &Exporter{
		URI:   uri,
		fetch: fetch,
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Was the last scrape of nginx successful.",
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_total_scrapes",
			Help:      "Current total nginx scrapes.",
		}),
		pushStreamMetrics: selectedPushStreamMetrics,
	}, nil
}

// Describe describes all the metrics ever exported by the nginx exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range e.pushStreamMetrics {
		ch <- m
	}
	ch <- nginxUp
	ch <- e.totalScrapes.Desc()
}

// Collect fetches the stats from configured nginx location and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	up := e.scrape(ch)
	ch <- prometheus.MustNewConstMetric(nginxUp, prometheus.GaugeValue, up)
	ch <- e.totalScrapes
}

func fetchHTTP(uri string, timeout time.Duration) func() (io.ReadCloser, error) {
	tr := &http.Transport{}
	client := http.Client{
		Timeout:   timeout,
		Transport: tr,
	}

	return func() (io.ReadCloser, error) {
		resp, err := client.Get(uri)
		if err != nil {
			return nil, err
		}
		if !(resp.StatusCode >= 200 && resp.StatusCode < 300) {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
		}

		return resp.Body, nil
	}
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) (up float64) {
	var subscribersTotal int64

	e.totalScrapes.Inc()

	body, err := e.fetch()

	if err != nil {
		log.Errorf("Can't scrape Nginx PushStream: %v", err)
		return 0
	}
	defer body.Close()

	ps := pushstream.NewPushStream()
	jsonError := json.NewDecoder(body).Decode(&ps)

	if jsonError != nil {
		log.Errorf("Unexpected error while reading JSON: %v", jsonError)
		return 0
	}

	v := reflect.ValueOf(ps).Elem()

	for key, val := range e.pushStreamMetrics {
		for i := 0; i < v.NumField(); i++ {
			valueField := v.Field(i)
			typeField := v.Type().Field(i)
			switch valueField.Interface().(type) {
			case int64:
				if key == typeField.Tag.Get("json") {
					ch <- prometheus.MustNewConstMetric(val, prometheus.GaugeValue, float64(valueField.Int()), "all")
				}
			case []*pushstream.Channel:
				for _, info := range ps.Infos {
					infoValue := reflect.ValueOf(info).Elem()

					for e := 0; e < infoValue.NumField(); e++ {
						valueInfoField := infoValue.Field(e)
						typeInfoField := infoValue.Type().Field(e)
						if key == typeInfoField.Tag.Get("json") {
							ch <- prometheus.MustNewConstMetric(val, prometheus.GaugeValue, float64(valueInfoField.Int()), info.Channel)
						}

						if key == "subscribers_total" && typeInfoField.Tag.Get("json") == "subscribers" {
							switch valueInfoField.Interface().(type) {
							case string:
								if n, err := strconv.Atoi(valueInfoField.String()); err == nil {
									subscribersTotal += int64(n)
								} else {
									fmt.Println(v, "is not an integer.")
									subscribersTotal += 0
								}
							case int64:
								subscribersTotal += valueInfoField.Int()
							}
						}
					}
				}
			}
		}
	}

	ch <- prometheus.MustNewConstMetric(e.pushStreamMetrics["subscribers_total"], prometheus.GaugeValue, float64(subscribersTotal), "all")

	return 1
}

// filterMetrics returns the set of pushStream metrics specified by the comma
// separated filter.
func filterMetrics(filter string) map[string]*prometheus.Desc {
	metrics := map[string]*prometheus.Desc{}
	if len(filter) == 0 {
		return metrics
	}

	selected := map[string]struct{}{}
	for _, f := range strings.Split(filter, ",") {
		selected[f] = struct{}{}
	}

	for field, metric := range pushStreamMetrics {
		if _, ok := selected[field]; ok {
			metrics[field] = metric
		}
	}
	return metrics
}

func main() {
	var (
		listenAddress     = flag.String("web.listen-address", ":9101", "Address to listen on for web interface and telemetry.")
		metricsPath       = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		nginxScrapeURI    = flag.String("nginx.scrape-uri", "http://localhost:8080/channels-stats?id=ALL", "URI on which to scrape Nginx PushStream channel stats.")
		nginxMetricFields = flag.String("nginx.metric-fields", pushStreamMetrics.String(), "Comma-separated list of exported server metrics.")
		nginxTimeout      = flag.Duration("nginx.timeout", time.Duration(5*time.Second), "Timeout for trying to get stats from nginx.")
		//nginxPidFile            = kingpin.Flag("nginx.pid-file", pidFileHelpText).Default("").String()
	)

	flag.Parse()
	log.Infoln("Starting nginx_pushstream_exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())

	selectedPushStreamMetrics := filterMetrics(*nginxMetricFields)

	exporter, err := NewExporter(*nginxScrapeURI, selectedPushStreamMetrics, *nginxTimeout)
	if err != nil {
		log.Fatal(err)
	}
	prometheus.MustRegister(exporter)
	prometheus.MustRegister(version.NewCollector("nginx_push_stream_exporter"))

	log.Infoln("Listening on", *listenAddress)
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Nginx PushStream Exporter</title></head>
             <body>
             <h1>Nginx PushStream Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
