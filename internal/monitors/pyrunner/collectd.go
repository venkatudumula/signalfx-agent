package pyrunner


type CollectdCore struct {
	runner *Runner
}

func (cc *CollectdCore) configureInRunner(conf config.MonitorCustomConfig) error {
	monitorID := conf.MonitorConfigCore().MonitorID
	cc.runner.Configure(
}
