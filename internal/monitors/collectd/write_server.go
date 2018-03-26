package collectd

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	log "github.com/sirupsen/logrus"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/metricproxy/protocol/collectd"
	"github.com/signalfx/signalfx-agent/internal/monitors/types"
	"github.com/signalfx/signalfx-agent/internal/utils"
)

// WriteServer is what receives value lists and notifiations from collectd and
// converts them to SignalFx datapoints and events.  Much of it is a
// reimplementation of what the metricproxy collectd endpoint does.  The main
// difference from metric proxy is that we propagate the meta field from
// collectd datapoints onto the resulting datapoints so that we can correlate
// metrics from collectd to specific monitors in the agent.  The server will
// run on the configured localhost port.
type WriteServer struct {
	ipAddr         string
	port           uint16
	activeMonitors map[types.MonitorID]types.Output
	httpServer     *http.Server
	udpServer      *net.UDPConn
	lock           sync.Mutex
}

// NewWriteServer creates but does not start a new write server
func NewWriteServer(ipAddr string, port uint16) (*WriteServer, error) {
	inst := &WriteServer{
		ipAddr:         ipAddr,
		port:           port,
		activeMonitors: make(map[types.MonitorID]types.Output),
	}

	return inst, nil
}

// Start begins accepting connections on the write server.  Will return an
// error if it cannot bind to the configured port.
func (s *WriteServer) Start() error {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.ParseIP(s.ipAddr),
		Port: int(s.port),
	})
	if err != nil {
		return err
	}

	s.httpServer = &http.Server{
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 1 * time.Second,
	}

	s.httpServer.Handler = s
	go s.httpServer.Serve(listener)

	s.udpServer, err = net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP(s.ipAddr),
		Port: int(s.port),
	})
	if err != nil {
		return err
	}

	go s.readDatagrams()

	return nil
}

func (s *WriteServer) RegisterMonitor(id types.MonitorID, out types.Output) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.activeMonitors[id] = out
}

func (s *WriteServer) UnregisterMonitor(id types.MonitorID) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.activeMonitors, id)
}

// Shutdown stops the write server immediately
func (s *WriteServer) Shutdown() error {
	s.httpServer.Close()
	s.udpServer.Close()
	// Don't worry about errors in the above calls
	return nil
}

// ServeHTTP accepts collectd write_http requests and sends the resulting
// datapoint/events to the configured callback functions.
func (s *WriteServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var writeBody collectd.JSONWriteBody
	err := json.NewDecoder(req.Body).Decode(&writeBody)
	if err != nil {
		log.WithError(err).Error("Could not decode body of write_http request")
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	// This is yet another way that collectd plugins can tell the agent what
	// the monitorID is.  This is specifically useful for the notifications
	// emitted by the metadata plugin.
	monitorID := req.URL.Query().Get("monitorID")

	for i := range writeBody {
		dps, ev := convertWriteFormat(writeBody[i])
		if ev != nil {
			if monitorID != "" {
				ev.Properties["monitorID"] = monitorID
			}
			s.receiveEvents([]*event.Event{ev})
		}
		if dps != nil {
			if monitorID != "" {
				addMonitorIDToDatapoints(monitorID, dps)
			}
			s.receiveDPs(dps)
		}
	}

	// Ingest returns this response but write_http doesn't care if it's there
	// or not and seems to dump the responses to stdout every couple of minutes
	// for some reason, so it's better to not send.
	//rw.Write([]byte(`"OK"`))
}

func addMonitorIDToDatapoints(monitorID string, dps []*datapoint.Datapoint) {
	for i := range dps {
		if dps[i].Meta["monitorID"] == nil {
			dps[i].Meta["monitorID"] = monitorID
		}
	}
}

func convertWriteFormat(f *collectd.JSONWriteFormat) ([]*datapoint.Datapoint, *event.Event) {
	if f.Time != nil && f.Severity != nil && f.Message != nil {
		return nil, collectd.NewEvent(f, nil)
	}

	// Preallocate space for dps
	dps := make([]*datapoint.Datapoint, 0, len(f.Dsnames))
	for i := range f.Dsnames {
		if i < len(f.Dstypes) && i < len(f.Values) && f.Values[i] != nil {
			dp := collectd.NewDatapoint(f, uint(i), nil)
			dp.Meta = utils.StringInterfaceMapToAllInterfaceMap(f.Meta)

			var toAddDims map[string]string
			// This is a hacky way to allow us to blindly append a square
			// bracket encoded set of dimensions on the end of the
			// type_instance field and parse it out here, without having to
			// worry about whether it already has a square bracket encoded set
			// of dimensions on it.
			dp.Metric, toAddDims = collectd.GetDimensionsFromName(&dp.Metric)
			for k, v := range toAddDims {
				if _, ok := dp.Dimensions[k]; !ok {
					dp.Dimensions[k] = v
				}
			}

			dps = append(dps, dp)
		}
	}

	return dps, nil
}

func (s *WriteServer) receiveDPs(dps []*datapoint.Datapoint) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for i := range dps {
		var monitorID types.MonitorID
		if id, ok := dps[i].Meta["monitorID"].(string); ok {
			monitorID = types.MonitorID(id)
		} else if id := dps[i].Dimensions["monitorID"]; id != "" {
			monitorID = types.MonitorID(id)
			delete(dps[i].Dimensions, "monitorID")
		}

		if string(monitorID) == "" {
			log.WithFields(log.Fields{
				"monitorID": monitorID,
				"datapoint": dps[i],
			}).Error("Datapoint does not specify its monitor id, cannot process")
			continue
		}

		output := s.activeMonitors[monitorID]
		if output == nil {
			log.WithFields(log.Fields{
				"monitorID": monitorID,
				"datapoint": dps[i],
			}).Error("Datapoint has an unknown monitorID")
			continue
		}

		output.SendDatapoint(dps[i])
	}
}

func (s *WriteServer) receiveEvents(events []*event.Event) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for i := range events {
		var monitorID types.MonitorID
		if id, ok := events[i].Properties["monitorID"].(string); ok {
			monitorID = types.MonitorID(id)
			delete(events[i].Properties, "monitorID")
		} else if id := events[i].Dimensions["monitorID"]; id != "" {
			monitorID = types.MonitorID(id)
			delete(events[i].Dimensions, "monitorID")
		}

		if string(monitorID) == "" {
			log.WithFields(log.Fields{
				"event": spew.Sdump(events[i]),
			}).Error("Event does not have a monitorID as either a dimension or property field, cannot send")
			continue
		}

		output := s.activeMonitors[monitorID]
		if output == nil {
			log.WithFields(log.Fields{
				"monitorID": monitorID,
			}).Error("Event has an unknown monitorID, cannot send")
			continue
		}

		output.SendEvent(events[i])
	}
}
