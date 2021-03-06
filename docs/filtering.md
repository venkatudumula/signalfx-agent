# Datapoint Filtering

To reduce your DPM (datapoints per minute), you may wish to filter out certain
datapoints at a high level to prevent them from ever leaving the agent.
Filtering can also be useful to reduce clutter in charts without having to
resort to filtering in the UI.  If possible, it is preferable to prevent the
datapoints you want to omit from being generated by a monitor in the first
place, as this will reduce CPU and memory usage of the agent, but sometimes
this is impractical.

Filters can be specified with the `metricsToExclude` config of the agent
config.  This parameter should be a list of filters that specify the metric
name and/or dimensions to match on.

Metric names and dimension values can be either globbed (i.e. where `*` is a
wildcard for zero or more characters, and `?` is a wildcard for a single
character) or specified as a Go-compatible regular expression (the value must
be surrounded by `/` to be considered a regex).

You can limit the scope of a filter to a specific monitor type with the
`monitorType` option in the filter item.  This value matches the monitor type
used in the monitor config.  This is very useful when trying to filter on a
heavily overloaded dimension, such as the `plugin_instance` dimension that most
collectd-based monitors emit.

Sometimes it is easier to whitelist the metrics you want to allow through.
You can do this by setting the `negated` option to `true` on a filter item.
This makes all metric name and dimension matching negated so that only
datapoints with that name or dimension are allowed through.  The `monitorType`
option is never negated, and always serves to scope a filter item to the
specified monitor type, if given.

Examples:

```yaml
  # If a metric matches any one of these filters it will be excluded.
  metricsToExclude:
    # Excludes all metrics that start with 'container_spec_memory_'
    - metricName: container_spec_memory_*

    # Excludes multiple metric names in a single filter
    - metricNames:
      - container_start_time_seconds
      - container_tasks_state

    # Excludes networking metrics about flannel interfaces that match the
    # pattern (e.g. 'flannel.1', 'flannel.2', etc.)
    - metricName: pod_network_*
      dimensions:
        interface: /flannel\.\d/

    # This is a metric from collectd.  All instances of it will be suppressed.
    - metricName: vmpage_io.swap.in

    # This filter will only match against datapoints from the collectd/df
    # monitor
    - dimensions:
        plugin_instance: dm-*
      monitorType: collectd/df

    # This only allows cpu and memory metrics from the docker monitor to come
    # through.  Note that the monitorType is never negated, and metrics from
    # other monitors will not be excluded since this filter is scoped to a
    # single monitor.
    - metricNames:
      - cpu*
      - memory*
      monitorType: collectd/docker
      negated: true
```
