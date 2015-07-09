package main

// based on https://github.com/y-matsuwitter/mackerel-fluentd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	mp "github.com/mackerelio/go-mackerel-plugin"
)

var Options struct {
	Host     string
	Port     int
	Tempfile string
}

type Plugin struct {
	Target  string
	Metrics []FluentMetric
	Err     error
}

type FluentMetric struct {
	Id                    string            `json:"plugin_id"`
	Category              string            `json:"plugin_category"`
	Type                  string            `json:"type"`
	Config                map[string]string `json:"config"`
	Output                bool              `json:"output_plugin"`
	RetryCount            int64             `json:"retry_count"`
	BufferQueueLength     int64             `json:"buffer_queue_length"`
	BufferTotalQueuedSize int64             `json:"buffer_total_queued_size"`
}

type FluentMetrics struct {
	Metrics []FluentMetric `json:"plugins"`
}

func (p Plugin) fetchFluentMetrics() ([]FluentMetric, error) {
	resp, err := http.Get(p.Target)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var metrics FluentMetrics
	err = json.Unmarshal(body, &metrics)
	if err != nil {
		return nil, err
	}
	return metrics.Metrics, nil
}

func (p *Plugin) Prepare() {
	p.Metrics, p.Err = p.fetchFluentMetrics()
}

func (p Plugin) FetchMetrics() (map[string]float64, error) {
	if p.Err != nil {
		return nil, p.Err
	}

	results := make(map[string]float64)
	for _, metric := range p.Metrics {
		if isEnableFluentMetric(metric) {
			results["retry."+metric.Id] = float64(metric.RetryCount)
			results["queue."+metric.Id] = float64(metric.BufferQueueLength)
			results["size."+metric.Id] = float64(metric.BufferTotalQueuedSize)
		}
	}
	return results, nil
}

func (p Plugin) GraphDefinition() map[string](mp.Graphs) {
	metrics := make([]mp.Metrics, 0)

	for _, metric := range p.Metrics {
		if isEnableFluentMetric(metric) {
			metrics = append(metrics, mp.Metrics{
				Name:  "retry." + metric.Id,
				Label: "Retry Count " + metric.Id,
				Diff:  false,
			})
			metrics = append(metrics, mp.Metrics{
				Name:  "queue." + metric.Id,
				Label: "Queue Length " + metric.Id,
				Diff:  false,
			})
			metrics = append(metrics, mp.Metrics{
				Name:  "size." + metric.Id,
				Label: "Buffer Size " + metric.Id,
				Diff:  false,
			})
		}
	}

	graphs := make(map[string]mp.Graphs)
	graphs["fluentd.buffer"] = mp.Graphs{
		Label:   "Fluentd Buffer",
		Metrics: metrics,
	}
	return graphs
}

func isEnableFluentMetric(m FluentMetric) bool {
	if !m.Output {
		return false
	}
	if strings.HasPrefix(m.Id, "object:") {
		return false
	}
	return true
}

func main() {
	flag.StringVar(&Options.Host, "host", "localhost", "fluentd monitor_agent host")
	flag.IntVar(&Options.Port, "port", 24220, "fluentd monitor_agent port")
	flag.StringVar(&Options.Tempfile, "tempfile", "", "Temp file name")
	flag.Parse()

	plugin := Plugin{
		Target: fmt.Sprintf("http://%s:%d/api/plugins.json", Options.Host, Options.Port),
	}
	plugin.Prepare()

	helper := mp.NewMackerelPlugin(plugin)

	if Options.Tempfile != "" {
		helper.Tempfile = Options.Tempfile
	} else {
		helper.Tempfile = fmt.Sprintf("/tmp/mackerel-plugin-fluentd-%s-%d", Options.Host, Options.Port)
	}

	if os.Getenv("MACKEREL_AGENT_PLUGIN_META") != "" {
		helper.OutputDefinitions()
	} else {
		helper.OutputValues()
	}
}
