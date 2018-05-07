"""
Python monitor runner
"""
from __future__ import absolute_import
import json
import logging
from functools import partial as p

from .logging import format_exception
from .collectd import CollectdMonitorProxy
from .messages import PipeMessageReader, PipeMessageWriter

logger = logging.getLogger()

class Runner(object):
    def __init__(self, input_reader, output_writer):
        # A set of monitor proxies keyed by monitor id
        self.monitor_proxies = {}

        assert isinstance(input_reader, PipeMessageReader)
        self.input_reader = input_reader

        assert isinstance(output_writer, PipeMessageWriter)
        self.output_writer = output_writer

    def run(self):
        """
        Starts the runner and blocks indefinitely
        """
        while True:
            self.read_control_message()

    def read_control_message(self):
        """
        Continuously read control messages that come in and create monitor
        proxies.  This method blocks until a control message arrives and is
        processed.
        """
        logger.info("Waiting for control messages")

        msg = self.input_reader.recv_msg()
        logger.debug("Received control message: %s", msg)

        if msg is None:
            return

        control_msg = json.loads(msg)
        mon_id = control_msg['monitorId']
        if control_msg["type"] == "configure":
            mon_proxy = None
            error = None
            try:
                mon_proxy = self.create_and_configure_new(
                    mon_id, control_msg['adapterType'], control_msg['config'])
            except (KeyboardInterrupt, SystemExit):
                raise
            except Exception as e:
                error = format_exception()

            if mon_proxy:
                self.monitor_proxies[mon_id] = mon_proxy

            self.output_writer.send_msg("configure_result", json.dumps({
                "monitorId": mon_id,
                "error": error,
            }))
        elif control_msg["type"] == "shutdown":
            self.shutdown_proxy(mon_id)
            self.output_writer.send_msg("shutdown_result", json.dumps({
                "monitorId": mon_id,
                "error": None,
            }))
        else:
            logging.error("Unknown control message: %s", control_msg)

    def create_and_configure_new(self, monitor_id, adapter_type, config):
        """
        Create and call configure on a new monitor proxy
        """
        proxy = None
        datapoint_send_func = p(self.send_datapoints, monitor_id)

        if adapter_type == 'collectd':
            proxy = CollectdMonitorProxy(datapoint_send_func)
        else:
            raise NotImplementedError("Unknown adapter type: %s" % (adapter_type,))

        # The configure method should raise an exception is there was an error
        proxy.configure(config)
        self.monitor_proxies[monitor_id] = proxy

    def send_datapoints(self, monitor_id, datapoints):
        self.output_writer.send_msg("datapoints", json.dumps(datapoints))

    def shutdown_proxy(self, monitor_id):
        """
        Shutdown and remove a monitor proxy
        """
        if monitor_id not in self.monitor_proxies:
            logging.warning("Monitor %s was not registered, cannot shutdown",
                            monitor_id)
            return

        try:
            self.monitor_proxies[monitor_id].shutdown()
        except (KeyboardInterrupt, SystemExit):
            raise
        except Exception:
            logging.error("Error shutting down monitor: %s",
                          format_exception())

        del self.monitor_proxies[monitor_id]
