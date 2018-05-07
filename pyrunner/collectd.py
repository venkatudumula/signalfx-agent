"""
Collectd's Python plugins are fairly difficult to work with.  There is no
first-class concept of multiple "instances" of a plugin, only one plugin module
that registers a single config callback that then gets invoked for each
configuration of the plugin.  Some plugins rely on a single read callback that
gets its configuration from a global variable and some create separate read
callbacks upon each invocation of the config callback.  Moreover, some plugins
don't handle multiple configurations at all and just overwrite the previous
configuration if the config callback is called multiple times.

There is also no concept of a plugin being "unconfigured", where a certain
configuration is removed.  There is a "shutdown" callback, but this is
dependent on the plugin implementing it properly. For these reasons, it is
virtually impossible to generically support dynamic monitor creation and
shutdown using the same Python interpreter.  Therefore, it is best if we simply
run each instance of the collectd-based monitor in a separate Python
interpreter, which means launching multiple instances of this Python adapter
process.

PEP 554 (https://www.python.org/dev/peps/pep-0554) will be incredibly useful
for us if it ever gets made a standard and we move to Python 3 since it would
allow for loading modules multiple times with totally isolated configurations of
the same plugin.  If a monitor associated with a particular config shutdown,
then the subinterpreter could just be destroyed.
"""
from __future__ import absolute_import
from functools import partial
import importlib
import logging
import sys
import types

from .proxy import MonitorProxy
from .scheduler import IntervalScheduler

logger = logging.getLogger()


def convert_value_to_datapoints(value_list):
    """
    Convert a collectd Values object to a set of datapoints.  We currently
    don't support multiple values, however, since that would require getting
    into types.db handling.
    """
    assert len(value_list.values) < 2, \
        "Multiple values in Collectd value lists are not supported at this time"

    return []


class Values(object):
    """
    Implementation of the Values object in collectd-python

    See https://collectd.org/documentation/manpages/collectd-python.5.shtml#values
    """
    def __init__(self, type=None, values=None, host=None, plugin=None,
                 plugin_instance=None, time=None, type_instance=None,
                 interval=None, meta=None):
        self.type = type
        self.values = values or []
        self.host = host
        self.plugin = plugin
        self.plugin_instance = plugin_instance
        self.time = time
        self.type_instance = type_instance
        self.interval = interval
        self.meta = meta or {}

    def dispatch(self):
        """
        This method technically can accept any of the kwargs accepted by the
        constructor but we'll defer supporting that until needed
        """
        logging.debug("Dispatching value %s", self.__dict__)
        dps = convert_value_to_datapoints(self)
        Values._dispatcher_func(dps)
        return

    @classmethod
    def set_dispatcher_func(cls, func):
        """
        Called to inject the function that sends datapoints when the `dispatch`
        method is called on a Values instance
        """
        cls._dispatcher_func = func


def inject_collectd_module(interface, send_datapoints_func):
    """
    Creates and registers the collectd python module so that plugins can import
    it properly.  This should only be called once per python interpreter.
    """
    assert 'collectd' not in sys.modules, 'collectd module should only be created once'

    mod = types.ModuleType('collectd')
    mod.register_config = interface.register_config
    mod.register_init = interface.register_init
    mod.register_read = interface.register_read

    Values.set_dispatcher_func(send_datapoints_func)
    mod.Values = Values
    sys.modules['collectd'] = mod


def load_plugin_module(module_paths, import_name):
    """
    Imports a collectd plugin module.  This will only have effect the first
    time it is called and subsequent calls will do nothing.
    """
    assert isinstance(module_paths, (tuple, list))

    for path in reversed(module_paths):
        if path not in sys.path:
            sys.path.insert(1, path)

    importlib.import_module(import_name)


class Config(object):
    """
    Dummy class that we use to put config that conforms to the collectd-python
    Config class

    See https://collectd.org/documentation/manpages/collectd-python.5.shtml#config
    """
    def __init__(self, root=None, key=None, values=None, children=None):
        self.root = root
        self.key = key
        self.values = values
        self.children = children

    @classmethod
    def from_monitor_config(cls, monitor_plugin_config):
        """
        Converts config as expressed in the monitor to the Collectd Config
        interface.
        """
        assert isinstance(monitor_plugin_config, dict)

        conf = cls(root=None)
        conf.children = []
        for k, v in monitor_plugin_config.items():
            values = None
            children = None
            if isinstance(v, (tuple, list)):
                values = v
            elif isinstance(v, (int, str, unicode)):
                values = (v,)
            elif isinstance(v, dict):
                dict_conf = cls.from_monitor_config(v)
                children = dict_conf.children
                values = dict_conf.values
            else:
                logging.error("Cannot convert monitor config to collectd config: %s: %s", k, v)

            conf.children.append(cls(root=conf, key=k, values=values, children=children))

        return conf

class CollectdMonitorProxy(MonitorProxy):
    """
    This is roughly analogous to a Monitor struct in the agent
    """
    def __init__(self, *args):
        super(CollectdMonitorProxy, self).__init__(*args)

        self.interface = None
        # The config that comes from the agent
        self.config = None

    def configure(self, monitor_config):
        """
        Calls the config callback with the given config, properly converted to
        the object that the collectd plugin expects.
        """
        assert 'pluginConfig' in monitor_config, \
            "Monitor config for collectd python should have a field called 'pluginConfig'"

        self.interface = CollectdInterface(monitor_config['intervalSeconds'])

        inject_collectd_module(self.interface, self.send_datapoints_func)

        module_paths = monitor_config.get('modulePaths', [])
        import_name = monitor_config['moduleName']

        load_plugin_module(module_paths, import_name)

        if not self.interface.config_callback:
            logging.error("No config callback was registered, cannot configure")
            return

        collectd_config = Config.from_monitor_config(monitor_config['pluginConfig'])
        self.config = monitor_config

        self.interface.config_callback(collectd_config)

    def shutdown(self):
        if self.interface:
            self.interface.do_shutdown()


class CollectdInterface(object):
    """
    This ultimately exposes and implements all of the collectd interface to the
    Python plugins.

    It can only handle a single plugin and unified set of plugin configuration.
    """
    def __init__(self, default_interval):
        assert isinstance(default_interval, int), 'default_interval must be an integer'
        self.default_interval = default_interval
        self.config_callback = None
        self.shutdown_callback = None
        self.read_callbacks = []
        self.shutdown_callbacks = []
        self.scheduler = IntervalScheduler()

    def register_config(self, callback):
        """
        Implementation of the register_config function from collectd
        """
        if self.config_callback:
            logging.warning("Config callback was already registered, re-registering")

        self.config_callback = callback

    def register_read(self, callback, interval=None, data=None, name=None):
        """
        Implementation of the register_read function, immediately schedules the
        read callback to run on the determined interval.
        """
        if data:
            func = partial(callback, data=data)
        else:
            func = callback

        self.scheduler.run_on_interval(interval or self.default_interval, func, immediately=True)

    def register_init(self, callback):
        """
        Just go ahead and call the init callback right away when registered
        since we don't have any interpreter setup to worry about by that point
        """
        callback()

    def register_shutdown(self, callback):
        self.shutdown_callback = callback

    def do_shutdown(self):
        self.scheduler.shutdown()

        if self.shutdown_callback:
            self.shutdown_callback()
