// +build ignore

// Package neopy holds the logic for managing Python plugins using a subprocess
// running Python. Currently there is only a DataDog monitor type, but we will
// support collectd Python plugins.  Communiation between this program and the
// Python runner is done through ZeroMQ IPC sockets.
//
// These Python monitors are configured the same as other monitors.
package neopy

import (
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/signalfx-agent/internal/monitors/types"
	log "github.com/sirupsen/logrus"
)

// Runner is the adapter to the Python monitor runner process.  It
// communiates with Python using named pipes.  Each general type of Python
// plugin (e.g. Datadog, collectd, etc.) should get its own generic monitor
// struct that uses this adapter.
type Runner struct {
	pythonProc       *exec.Cmd
	cancel func()

	inputToPython pipeMessageWriter
	outputFromPython pipeMessageReader
}

func New() *Runner {
	return &Runner{
	}
}

func (r *Runner) run() error {
	log.Info("Starting Runner")
	ctx, r.cancel = context.WithCancel(context.Background())

	cmd := r.makeChildCommand(ctx)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	r.inputToPython = pipeMessageWriter(stdin)
	r.outputFromPython = pipeMessageReader(stdout)

	// Stderr is just the normal output from the Python code that isn't
	// specially encoded
	r.stderr, err = cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func(){
		if err := cmd.Wait(); err != nil {
			log.WithError(err).Error("Python runner process crashed")
			if r.cancel != nil {
				r.cancel()
			}
		}
	}()

	return nil
}

// Configure could be used later for passing config to the Runner instance.
// Right now it is just used to start up the child process if not running.
func (r *Runner) Configure() bool {
	if r.subproc == nil {
		r.start()
	}
	return true
}

func (r *Runner) sendDatapointsForMonitorTo(monitorID types.MonitorID, dpChan chan<- *datapoint.Datapoint) {
	r.dpChannelsLock.Lock()
	defer r.dpChannelsLock.Unlock()
	r.dpChannels[monitorID] = dpChan
}

func (r *Runner) sendDatapointsToMonitors() {
	for {
		select {
		case dpMessage := <-r.mainDPChan:
			r.dpChannelsLock.RLock()

			if ch, ok := r.dpChannels[dpMessage.MonitorID]; ok {
				ch <- dpMessage.Datapoint
			} else {
				log.WithFields(log.Fields{
					"dpMessage": dpMessage,
				}).Error("Could not find monitor ID to send datapoint from Runner")
			}

			r.dpChannelsLock.RUnlock()
		}
	}
}

// ConfigureInPthon sends the given config to the python subproc and returns
// whether configuration was successful
func (r *Runner) ConfigureInPython(config interface{}) bool {
	return r.configQueue.configure(config)
}

// ShutdownMonitor will shutdown the given monitor id in the python subprocess
func (r *Runner) ShutdownMonitor(monitorID types.MonitorID) {
	r.sendMsg("shutdown", 
	delete(r.dpChannels, monitorID)
}

// Shutdown the whole Runner child process, not just individual monitors
func (r *Runner) Shutdown() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Runner) makeChildCommand(ctx context.Context) (*exec.Cmd) {
	log.Info("Starting Python runner child process")
	cmd := exec.CommandContext(ctx, "python", "-m", "pyrunner")

	return cmd
}
