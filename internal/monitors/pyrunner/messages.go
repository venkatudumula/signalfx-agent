// +build ignore

package neopy

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
)

type RunnerCommand interface {
	Type() string
}

type ConfigureCommand struct {
	MonitorId int `json:"monitorId"`
	// The type of the proxied monitor (e.g. collectd, datadog, etc)
	AdapterType string `json:"adapterType"`
	// A path to a pipe/fifo in the filesystem that should be used to send back
	// datapoints from the Python monitor.
	DatapointFifo string `json:"datapointFifo"`
	// The configuration for the proxied monitor
	Config interface{} `json:"config"`
}

func (c *ConfigureCommand) Type() {
	return "configure"
}

type ShutdownCommand struct {
	MonitorId int `json:"monitorId"`
}

func (s *ShutdownCommand) Type() {
	return "shutdown"
}

// ControlResponse is what we expect to be sent by Python to indicate whether
// a control request was successful or not.
type CommandResponse struct {
	MonitorId int     `json:"monitorId"`
	Type      string  `json:"type"`
	Error     *string `json:"error"`
}

// ControlPipePair is a pair of pipes used to send configuration and shutdown
// messages to the python runner and get back whether the operation was
// successful or not.
type CommandSender struct {
	requestPipe  PipeWriter
	responsePipe PipeReader
}

func newCommandSender(input *os.File, output *os.File) *CommandSender {
	return &CommandSender{
		requestPipe: PipeWriter{
			file: input,
		},
		responsePipe: PipeReader{
			file: output,
		},
	}
}

func (cq *ControlPipePair) configure(conf interface{}) error {
	confJSON, err := json.Marshal(conf)
	if err != nil {
		errors.Wrap(err, "Could not serialize monitor config to JSON")
	}

	return cp.sendRequest(confJSON, "configure")
}

func (cq *ControlPipePair) shutdown() error {
	confJSON, err := json.Marshal(conf)
	if err != nil {
		errors.Wrap(err, "Could not serialize monitor config to JSON")
	}

	return cp.sendRequest(confJSON, "configure")
}

func (cp *ControlPipePair) sendRequest(msg []byte, typ string) error {
	err = cp.requestPipe.SendMessage(msg)
	if err != nil {
		return errors.Wrapf(err, "Could not send message of type %s to Python", typ)
	}

	respJSON, err := cp.responsePipe.RecvMessageBytes()
	if err != nil {
		return errors.Wrapf(err,
			"Failed getting response for control message of type %s from Python runner", typ)
	}

	var resp ControlResponse
	err = json.Unmarshal(respJSON, &resp)
	if err != nil {
		return errors.Wrapf(err, "Could not parse Python response for control request of type %s", typ)
	}

	if resp.Error != nil {
		return errors.New(resp.Error)
	}

	return nil
}
