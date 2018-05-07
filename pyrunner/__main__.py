from __future__ import absolute_import
import bdb
import logging
import logging.config
import os
import sys

from .messages import PipeMessageReader, PipeMessageWriter
from .runner import Runner
from .logging import log_exc_traceback_as_error, PipeLogHandler

logging.config.dictConfig({
    "version": 1,
    "formatters": {},
    "filters": {},
    "handlers": {},
    "loggers": {},
})

logger = logging.getLogger()
logger.setLevel(logging.DEBUG)


def replace_stdout():
    """
    Returns a new fd that is connected to the original stdout of the process.
    The new stdout will be connected to stderr so that all process output using
    the standard stdout fd goes there.
    """
    stdout_copy = os.dup(sys.stdout.fileno())
    os.dup2(sys.stderr.fileno(), sys.stdout.fileno())

    return stdout_copy

real_out_fd = replace_stdout()
# Let stderr continue to be directed to the original stderr fd

# This process sends data back to the agent through stdout
output_writer = PipeMessageWriter(real_out_fd)
output_writer.open()

logger.addHandler(PipeLogHandler(output_writer))

# The agent sends control messages to this process via stdin
input_reader = PipeMessageReader(sys.stdin.fileno())
input_reader.open()

runner = Runner(input_reader, output_writer)

logger.info("Starting up Python monitor runner")

try:
    runner.run()
except (KeyboardInterrupt, SystemExit, bdb.BdbQuit):
    sys.exit(1)
except Exception as e:
    #runner.stop()
    log_exc_traceback_as_error()
