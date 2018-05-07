"""
Logging helpers
"""
from __future__ import absolute_import
import logging
import json
import sys
import traceback

logger = logging.getLogger()

class PipeLogHandler(logging.Handler):
    """
    Python log handler that converts log messages to a json object and sends
    them back to the agent through the given pipe
    """
    def __init__(self, pipe):
        """
        `pipe` should be a PipeMessageWriter that is already opened
        """
        self.pipe = pipe

        super(PipeLogHandler, self).__init__()

    def emit(self, record):
        self.pipe.send_msg("log", json.dumps({
            "message": record.msg,
            "logger": record.name,
            "source_path": record.pathname,
            "lineno": record.lineno,
            "created": record.created,
            "level": record.levelname,
        }))

def format_exception():
    """
    Format the current exception as a traceback
    """
    exc_type, exc_value, exc_traceback = sys.exc_info()
    return traceback.format_exception(exc_type, exc_value, exc_traceback)

def log_exc_traceback_as_error():
    """
    Log the current exception at the error level.  Meant to be called when in
    an except block
    """
    logger.error(format_exception())
