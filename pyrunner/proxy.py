"""
Logic for interfacing between the monitor in the agent and the python code that
gets metrics
"""

class MonitorProxy(object):
    """
    Acts as the proxy for the monitor in the agent and implements a
    similar interface
    """
    def __init__(self, datapoint_sender_func):
        self.send_datapoints_func = datapoint_sender_func

    def configure(self, monitor_config):
        """
        Proxy for the Configure method of agent monitors
        """
        raise NotImplementedError("configure method must be implemented by monitor subclass")

    def shutdown(self):
        """
        Proxy for the Shutdown method of agent monitors
        """
        raise NotImplementedError("shutdown method must be implemented by monitor subclass")
