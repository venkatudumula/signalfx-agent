// +build windows

package utilization

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/mem"
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/sfxclient"
	"github.com/signalfx/signalfx-agent/internal/monitors/telegraf/common/accumulator"
	"github.com/signalfx/signalfx-agent/internal/monitors/telegraf/common/emitter/batchemitter"
	"github.com/signalfx/signalfx-agent/internal/monitors/telegraf/common/measurement"
	"github.com/signalfx/signalfx-agent/internal/monitors/telegraf/monitors/winperfcounters"
	"github.com/signalfx/signalfx-agent/internal/utils"
)

// This maps the measurement name from telegraf to a desired metric name
var metricNameMapping = map[string]string{
	"Bytes_Received_persec":   "if_octets.rx",
	"Bytes_Sent_persec":       "if_octets.tx",
	"Packets_Received_Errors": "if_errors.rx",
	"Packets_Outbound_Errors": "if_errors.tx",
	"Percent_Free_Space":      "disk.utilization",
	"Disk_Reads_persec":       "disk_ops.read",
	"Disk_Writes_persec":      "disk_ops.write",
	"Page_Reads_persec":       "vmpage_io.swap.in.per.sec",
	"Page_Writes_persec":      "vmpage_io.swap.out.per.sec",
}

func (m *Monitor) emitMemoryUtilization() {
	memInfo, _ := mem.VirtualMemory()
	dpused := sfxclient.Gauge("memory.used", map[string]string{"plugin": "signalfx-metadata"}, int64(memInfo.Used))
	m.Output.SendDatapoint(dpused)
	dptotal := sfxclient.Gauge("memory.total", map[string]string{"plugin": "signalfx-metadata"}, int64(memInfo.Total))
	m.Output.SendDatapoint(dptotal)
	util := (float64(memInfo.Used) / float64(memInfo.Total)) * 100
	dputil := sfxclient.GaugeF("memory.utilization", map[string]string{"plugin": "signalfx-metadata"}, util)
	m.Output.SendDatapoint(dputil)
}

func (m *Monitor) processCPU(ms *measurement.Measurement) {
	dimensions := map[string]string{"plugin": "signalfx-metadata", "plugin_instance": "utilization"}
	metricName := "cpu.utilization"

	// handle cpu utilization per core if instance isn't _Total
	if instance, ok := ms.Tags["instance"]; ok && instance != "_Total" {
		metricName = "cpu.utilization_per_core"
		dimensions["core"] = instance
	}

	// parse metric value
	var metricVal datapoint.Value
	var err error
	if val, ok := ms.Fields["Percent_Processor_Time"]; ok {
		if metricVal, err = datapoint.CastMetricValue(val); err != nil {
			logger.Error(err)
			return
		}
	}

	// create datapoint
	dp := sfxclient.Gauge(metricName, dimensions, metricVal)
	m.Output.SendDatapoint(dp)
}

func (m *Monitor) processNetIO(ms *measurement.Measurement) {
	for field, val := range ms.Fields {
		dimensions := map[string]string{"plugin": "interface"}

		// set metric name
		metricName := metricNameMapping[field]
		if metricName == "" {
			logger.Errorf("unable to map field '%s' to a metricname while parsing measurement '%s'",
				field, ms.Measurement)
			continue
		}

		// parse metric value
		var metricVal datapoint.Value
		var err error
		if metricVal, err = datapoint.CastMetricValue(val); err != nil {
			logger.Error(err)
			continue
		}

		// set the instance dimension
		if instance, ok := ms.Tags["instance"]; ok {
			dimensions["plugin_instance"] = instance
		} else {
			logger.Errorf("no instance tag defined in tags '%v' for field '%s' on measurement '%s'",
				ms.Tags, field, ms.Measurement)
			continue
		}

		dp := datapoint.New(metricName, dimensions, metricVal, datapoint.Counter, time.Time{})
		m.Output.SendDatapoint(dp)
	}
}

func (m *Monitor) processDisk(ms *measurement.Measurement) {
	dimensions := map[string]string{"plugin": "signalfx-metadata"}

	// set metric name
	metricName := "disk.utilization"

	// set the instance dimension
	if instance, ok := ms.Tags["instance"]; ok {
		dimensions["plugin_instance"] = instance
	}

	// parse metric value
	var metricVal datapoint.Value
	if val, ok := ms.Fields["Percent_Free_Space"]; ok {
		if v, ok := val.(float32); ok {
			// disk.utilization is 100-%free
			metricVal = datapoint.NewFloatValue(float64(100) - float64(v))
		} else {
			logger.Error("error parsing metric")
			return
		}
	}

	dp := datapoint.New(metricName, dimensions, metricVal, datapoint.Gauge, time.Now())
	m.Output.SendDatapoint(dp)
}

func (m *Monitor) processPhysDisk(ms *measurement.Measurement) {
	for field, val := range ms.Fields {
		dimensions := map[string]string{"plugin": "disk"}

		// set metric name
		metricName := metricNameMapping[field]
		if metricName == "" {
			logger.Errorf("unable to map field '%s' to a metricname while parsing measurement '%s'",
				field, ms.Measurement)
			continue
		}

		// set the instance dimension
		if instance, ok := ms.Tags["instance"]; ok {
			dimensions["plugin_instance"] = instance
		} else {
			logger.Errorf("no instance tag defined in tags '%v' for field '%s' on measurement '%s'",
				ms.Tags, field, ms.Measurement)
			continue
		}

		// parse metric value
		var metricVal datapoint.Value
		var err error
		if metricVal, err = datapoint.CastMetricValue(val); err != nil {
			logger.Error(err)
			continue
		}

		dp := datapoint.New(metricName, dimensions, metricVal, datapoint.Gauge, time.Now())
		m.Output.SendDatapoint(dp)
	}
	// logger.Debug(ms)
}

func (m *Monitor) processMem(ms *measurement.Measurement) {
	for field, val := range ms.Fields {
		dimensions := map[string]string{"plugin": "vmem"}

		// set metric name
		metricName := metricNameMapping[field]
		if metricName == "" {
			logger.Errorf("unable to map field '%s' to a metricname while parsing measurement '%s'",
				field, ms.Measurement)
			continue
		}

		// parse metric value
		var metricVal datapoint.Value
		var err error
		if metricVal, err = datapoint.CastMetricValue(val); err != nil {
			logger.Error(err)
			continue
		}

		dp := datapoint.New(metricName, dimensions, metricVal, datapoint.Gauge, time.Now())
		m.Output.SendDatapoint(dp)
	}
	logger.Debug(ms)
}

// processMeasurments iterates over each measurement in the list and sends them to the appropriate
// function for parsing
func (m *Monitor) processMeasurements(measurements []*measurement.Measurement) {
	for _, measurement := range measurements {
		switch measurement.Measurement {
		case "win_cpu":
			m.processCPU(measurement)
		case "win_network_interface":
			m.processNetIO(measurement)
		case "win_disk":
			m.processDisk(measurement)
		case "win_physical_disk":
			m.processPhysDisk(measurement)
		case "win_mem":
			m.processMem(measurement)
		default:
			logger.Error("utilization plugin collected unknown measurement")
		}
	}
}

// Configure the monitor and kick off metric syncing
func (m *Monitor) Configure(conf *Config) error {
	perfcounterConf := &winperfcounters.Config{
		CountersRefreshInterval: conf.CountersRefreshInterval,
		PrintValid:              conf.PrintValid,
		Object: []winperfcounters.Perfcounterobj{
			winperfcounters.Perfcounterobj{
				ObjectName:   "Processor",
				Counters:     []string{"% Processor Time"},
				Instances:    []string{"*"},
				Measurement:  "win_cpu",
				IncludeTotal: true,
			},
			winperfcounters.Perfcounterobj{
				ObjectName: "Network Interface",
				Counters: []string{"Bytes Received/sec", "Bytes Sent/sec",
					"Packets Outbound Errors", "Packets Received Errors"},
				Instances:    []string{"*"},
				Measurement:  "win_network_interface",
				IncludeTotal: false,
			},
			winperfcounters.Perfcounterobj{
				ObjectName:   "LogicalDisk",
				Counters:     []string{"% Free Space"},
				Instances:    []string{"*"},
				Measurement:  "win_disk",
				IncludeTotal: false,
			},
			winperfcounters.Perfcounterobj{
				ObjectName:   "PhysicalDisk",
				Counters:     []string{"Disk Writes/sec", "Disk Reads/sec"},
				Instances:    []string{"*"},
				Measurement:  "win_physical_disk",
				IncludeTotal: false,
			},
			// memory.utilization needs some work
			winperfcounters.Perfcounterobj{
				ObjectName:   "Memory",
				Counters:     []string{"Page Reads/sec", "Page Writes/sec"},
				Instances:    []string{"------"},
				Measurement:  "win_mem",
				IncludeTotal: true,
			},
		},
	}
	plugin := winperfcounters.GetPlugin(perfcounterConf)

	// create the emitter
	emitter := batchemitter.NewEmitter(m.Output, logger)

	// create the accumulator
	ac := accumulator.NewAccumulator(emitter)

	// create contexts for managing the the plugin loop
	var ctx context.Context
	ctx, m.cancel = context.WithCancel(context.Background())

	// gather metrics on the specified interval
	utils.RunOnInterval(ctx, func() {
		if err := plugin.Gather(ac); err != nil {
			logger.Error(err)
			return
		}
		m.processMeasurements(emitter.Measurements)
		m.emitMemoryUtilization()

		// reset emitter measurements
		emitter.Measurements = emitter.Measurements[:0]

	}, time.Duration(conf.IntervalSeconds)*time.Second)

	return nil
}

// Shutdown stops the metric sync
func (m *Monitor) Shutdown() {
	if m.cancel != nil {
		m.cancel()
	}
}
