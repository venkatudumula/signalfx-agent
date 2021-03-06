<!--- GENERATED BY gomplate from scripts/docs/monitor-page.md.tmpl --->

# collectd/marathon

 Monitors a Mesos Marathon instance using the
[collectd Marathon Python plugin](https://github.com/signalfx/collectd-marathon).

See the [integrations
doc](https://github.com/signalfx/integrations/tree/master/collectd-marathon)
for more information on configuration.

Sample YAML configuration:

```yaml
monitors:
  - type: collectd/marathon
    host: 127.0.0.1
    port: 8080
    scheme: http
```

Sample YAML configuration for DC/OS:

```yaml
monitors:
  - type: collectd/marathon
    host: 127.0.0.1
    port: 8080
    scheme: https
    dcosAuthURL: https://leader.mesos/acs/api/v1/auth/login
```


Monitor Type: `collectd/marathon`

[Monitor Source Code](https://github.com/signalfx/signalfx-agent/tree/master/internal/monitors/collectd/marathon)

**Accepts Endpoints**: **Yes**

**Multiple Instances Allowed**: **No**

## Configuration

| Config option | Required | Type | Description |
| --- | --- | --- | --- |
| `host` | **yes** | `string` |  |
| `port` | **yes** | `integer` |  |
| `username` | no | `string` | Username used to authenticate with Marathon. |
| `password` | no | `string` | Password used to authenticate with Marathon. |
| `scheme` | no | `string` | Set to either `http` or `https`. (**default:** `http`) |
| `dcosAuthURL` | no | `string` | The dcos authentication URL which the plugin uses to get authentication tokens from. Set scheme to "https" if operating DC/OS in strict mode and dcosAuthURL to "https://leader.mesos/acs/api/v1/auth/login" (which is the default DNS entry provided by DC/OS) |




## Metrics

This monitor emits the following metrics.  Note that configuration options may
cause only a subset of metrics to be emitted.

| Name | Type | Description |
| ---  | ---  | ---         |
| `gauge.marathon-api-metric` | gauge | Metrics reported by the Marathon Metrics API |
| `gauge.marathon.app.cpu.allocated` | gauge | Number of CPUs allocated to an application |
| `gauge.marathon.app.cpu.allocated.per.instance` | gauge | Configured number of CPUs allocated to each application instance |
| `gauge.marathon.app.delayed` | gauge | Indicates if the application is delayed or not |
| `gauge.marathon.app.deployments.total` | gauge | Number of application deployments |
| `gauge.marathon.app.disk.allocated` | gauge | Storage allocated to a Marathon application |
| `gauge.marathon.app.disk.allocated.per.instance` | gauge | Configured storage allocated each to application instance |
| `gauge.marathon.app.gpu.allocated` | gauge | GPU Allocated to a Marathon application |
| `gauge.marathon.app.gpu.allocated.per.instance` | gauge | Configured number of GPUs allocated to each application instance |
| `gauge.marathon.app.instances.total` | gauge | Number of application instances |
| `gauge.marathon.app.memory.allocated` | gauge | Memory Allocated to a Marathon application |
| `gauge.marathon.app.memory.allocated.per.instance` | gauge | Configured amount of memory allocated to each application instance |
| `gauge.marathon.app.tasks.running` | gauge | Number tasks running for an application |
| `gauge.marathon.app.tasks.staged` | gauge | Number tasks staged for an application |
| `gauge.marathon.app.tasks.unhealthy` | gauge | Number unhealthy tasks for an application |
| `gauge.marathon.task.healthchecks.failing.total` | gauge | The number of failing health checks for a task |
| `gauge.marathon.task.healthchecks.passing.total` | gauge | The number of passing health checks for a task |
| `gauge.marathon.task.staged.time.elapsed` | gauge | The amount of time the task spent in staging |
| `gauge.marathon.task.start.time.elapsed` | gauge | Time elapsed since the task started |



