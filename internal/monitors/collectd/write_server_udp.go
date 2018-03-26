package collectd

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/metricproxy/protocol/collectd"
)

// Taken from src/network.h of collectd
const (
	TYPE_HOST            = 0x0000
	TYPE_TIME            = 0x0001
	TYPE_PLUGIN          = 0x0002
	TYPE_PLUGIN_INSTANCE = 0x0003
	TYPE_TYPE            = 0x0004
	TYPE_TYPE_INSTANCE   = 0x0005
	TYPE_VALUES          = 0x0006
	TYPE_INTERVAL        = 0x0007
	TYPE_TIME_HR         = 0x0008
	TYPE_INTERVAL_HR     = 0x0009
	TYPE_DS_NAME         = 0x000a
	TYPE_MESSAGE         = 0x0100
	TYPE_SEVERITY        = 0x0101
)

var (
	counterType  = "counter"
	gaugeType    = "gauge"
	deriveType   = "derive"
	absoluteType = "absolute"
)

var defaultTs = 0.0

// Takes a UDP datagram from the collectd network plugin and parses out
// datapoints and events.  This should use less CPU both in collectd and the
// agent compared to JSON over HTTP/TCP at a greater risk of losing metrics.
func (s *WriteServer) readDatagrams() {
	// This is the default of the network plugin in collectd.  If it gets
	// changed in the network plugin config, it must be changed here to ensure
	// no packets are truncated!  Most datagrams will probably not be nearly
	// this big.
	const maxDatagramSize = 1452
	const maxBacklogSize = 512
	const bufSize = maxDatagramSize * (maxBacklogSize + 1)
	dgrams := make(chan []byte, maxBacklogSize)

	// Only start up one processor so that we don't have to worry about
	// datapoint ordering getting mixed up.
	go s.processDatagrams(dgrams)

	buf := make([]byte, bufSize, bufSize)
	activeBuf := buf
	for {
		// We have very little control over how big the kernel UDP buffer is,
		// and it is critical that we never let it get filled, or else we will
		// lose metrics.  Usually the default read buffer size on the socket is
		// the same as the max buffer size allowed by
		// /proc/sys/net/core/rmem_max, so there is little point in setting it.
		// The best thing is to just keep pulling datagrams out as fast as
		// possible and hand them off to another goroutine to process via a
		// buffered channel.  Of course, if that channel gets full we have the
		// same problem, but we can control the channel size much more than the
		// UDP socket buffer size.

		// To help keep us reading datagrams as fast as possible, we make a big
		// buffer up front and reuse it from the beginning when we get to the
		// end.  That way, this loop never requires memory allocations.
		// Because the buffer size can hold one more maximally-sized datagram
		// than the channel buffer size, even if we hit the max backlog we
		// would not clobber bytes still in use by the processor.
		if len(activeBuf) < maxDatagramSize {
			activeBuf = buf
		}
		n, err := s.udpServer.Read(activeBuf)
		if err != nil {
			close(dgrams)
			return
		}

		dgrams <- activeBuf[:n]
		activeBuf = activeBuf[n:]
	}
}

func (s *WriteServer) processDatagrams(ch <-chan []byte) {
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// ch is closed so stop processing
				return
			}
			dps, events := parseDatagram(msg)
			if events != nil {
				s.receiveEvents(events)
			}
			if dps != nil {
				s.receiveDPs(dps)
			}
		}
	}
}

// See https://collectd.org/wiki/index.php/Binary_protocol for the spec.
func parseDatagram(msg []byte) ([]*datapoint.Datapoint, []*event.Event) {
	var dps []*datapoint.Datapoint
	var events []*event.Event
	var current *collectd.JSONWriteFormat
	var timestamp time.Time
	var done bool
	var isNotif bool
	for {
		if len(msg) < 4 {
			break
		}
		msgType, length := binary.BigEndian.Uint16(msg[:2]), binary.BigEndian.Uint16(msg[2:4])

		if current == nil {
			current = &collectd.JSONWriteFormat{
				// We'll override this after everything is made using the
				// high-res timestamp.
				Time: &defaultTs,
			}
		}

		switch msgType {
		case TYPE_HOST:
			current.Host = parseString(msg, length)
		case TYPE_PLUGIN:
			current.Plugin = parseString(msg, length)
		case TYPE_PLUGIN_INSTANCE:
			current.PluginInstance = parseString(msg, length)
		case TYPE_TYPE:
			current.TypeS = parseString(msg, length)
		case TYPE_TYPE_INSTANCE:
			current.TypeInstance = parseString(msg, length)
		case TYPE_TIME:
			timestamp = time.Unix(int64(binary.BigEndian.Uint64(msg[4:12])), 0)
		case TYPE_TIME_HR:
			// Collectd's high-res timestamp has a weird unit of 2^-30 seconds,
			// so convert that to nanoseconds.  Save it and override the
			// timestamp on the datapoint produced since the JSON converter
			// only supports 1 second resolution.
			timestamp = time.Unix(0, int64(float64(binary.BigEndian.Uint64(msg[4:12]))*(0.931322574615478515625)))
		case TYPE_DS_NAME:
			current.Dsnames = append(current.Dsnames, parseString(msg, length))
		case TYPE_SEVERITY:
			/*var severity string
			// Values from src/daemon/plugin.h
			switch binary.BigEndian.Uint64(msg[4:12]) {
			case 0x1:
				severity = "FAILURE"
			case 0x2:
				severity = "WARNING"
			case 0x4:
				severity = "OKAY"
			default:
				severity = "UNKNOWN"
			}
			current.Severity = &severity*/
			done = true
			isNotif = true
		case TYPE_MESSAGE:
			/*current.Message = parseString(msg, length)
			// Getting a message means we are done with an event, so finish it
			// up.
			*/
			done = true
			isNotif = true
		case TYPE_VALUES:
			current.Dstypes, current.Values = parseValues(msg)
			// When we see a set of values, we know the current value list is
			// done.
			done = true
		}

		if done {
			// The metadata plugin is responsible for sending notifications to
			// the http side of this so just ignore notifications here.
			if isNotif {
				isNotif = false
			} else {
				newDPs, _ := convertWriteFormat(current)
				if newDPs != nil {
					for i := range newDPs {
						newDPs[i].Timestamp = timestamp
					}
					dps = append(dps, newDPs...)
				}
			}

			current = nil
			done = false
		}

		msg = msg[length:]
	}
	return dps, events
}

func parseString(msg []byte, length uint16) *string {
	val := string(msg[4 : length-1])
	return &val
}

// Parse out the values entry.  The return types are to facilitate construction
// of the JSONWriteFormat struct that is used to produce a datapoint.
func parseValues(msg []byte) ([]*string, []*float64) {
	nValues := binary.BigEndian.Uint16(msg[4:6])
	types := make([]*string, nValues, nValues)
	values := make([]*float64, nValues, nValues)

	pos := 6
	for i := uint16(0); i < nValues; i++ {
		switch msg[pos] {
		case 0x0:
			types[i] = &counterType
		case 0x1:
			types[i] = &gaugeType
		case 0x2:
			types[i] = &deriveType
		case 0x3:
			types[i] = &absoluteType
		}
		pos++
	}

	for i := uint16(0); i < nValues; i++ {
		byteVal := msg[pos : pos+8]
		var val float64
		switch *types[i] {
		case counterType:
			fallthrough
		case absoluteType:
			val = float64(binary.BigEndian.Uint64(byteVal))
		case gaugeType:
			bits := binary.LittleEndian.Uint64(byteVal)
			val = math.Float64frombits(bits)
		case deriveType:
			val = float64(int64(binary.BigEndian.Uint64(byteVal)))
		}
		values[i] = &val
		pos += 8
	}
	return types, values
}
