package telegrafutil

import (
	"time"

	"github.com/influxdata/telegraf/plugins/outputs/signalfx/parse"
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/signalfx-agent/internal/monitors/types"
	log "github.com/sirupsen/logrus"
)

func getTime(t ...time.Time) time.Time {
	if len(t) > 0 {
		return t[0]
	} else {
		return time.Now()
	}
}

func getDims(fields map[string]string) (dims map[string]string) {
	dims = make(map[string]string, len(fields))
	for k, v := range fields {
		dims[k] = v
	}
	return
}

// Accumulator is an interface for "accumulating" metrics from plugin(s).
// The metrics are sent down a channel shared between all plugins.
type Accumulator struct {
	Output      types.Output
	MonitorType string
	Rename      map[string]string
	logger      *log.Entry
}

func (ac *Accumulator) emitMetric(measurement string, fields map[string]interface{},
	tags map[string]string, metricType datapoint.MetricType, metricTypeString string, t ...time.Time) {
	for field, val := range fields {
		var metricDims = getDims(tags)
		var props = make(map[string]interface{})
		var metricName = parse.GetMetricName(measurement, field, metricDims)
		_ = parse.ModifyDimensions(metricName, metricDims, props)
		if metricValue, err := datapoint.CastMetricValue(val); err == nil {
			var dp = datapoint.New(metricName,
				metricDims,
				metricValue.(datapoint.Value),
				metricType,
				getTime(t...))
			ac.Output.SendDatapoint(dp)
		} else {
			// Skip if it's not an sfx metric and it's not included
			if _, isSFX := tags["sf_metric"]; !isSFX && !s.isIncluded(metricName) {
				continue
			}
		}

	}
}

// AddFields adds a metric to the accumulator with the given measurement
// name, fields, and tags (and timestamp). If a timestamp is not provided,
// then the accumulator sets it to "now".
// Create a point with a value, decorating it with tags
// NOTE: tags is expected to be owned by the caller, don't mutate
// it after passing to Add.
func (ac *Accumulator) AddFields(measurement string, fields map[string]interface{},
	tags map[string]string, t ...time.Time) {

}

// AddGauge is the same as AddFields, but will add the metric as a "Gauge" type
func (ac *Accumulator) AddGauge(measurement string, fields map[string]interface{},
	tags map[string]string, t ...time.Time) {

}

// AddCounter is the same as AddFields, but will add the metric as a "Counter" type
func (ac *Accumulator) AddCounter(measurement string, fields map[string]interface{},
	tags map[string]string, t ...time.Time) {

}

// AddSummary is the same as AddFields, but will add the metric as a "Summary" type
func (ac *Accumulator) AddSummary(measurement string, fields map[string]interface{},
	tags map[string]string, t ...time.Time) {

}

// AddHistogram is the same as AddFields, but will add the metric as a "Histogram" type
func (ac *Accumulator) AddHistogram(measurement string, fields map[string]interface{},
	tags map[string]string, t ...time.Time) {

}

// SetPrecision -
func (ac *Accumulator) SetPrecision(precision, interval time.Duration) {
	return
}

// AddError -
func (ac *Accumulator) AddError(err error) {
	ac.logger.WithError(err)
}
