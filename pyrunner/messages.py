"""
Logic for communicating with FIFO pipes between Python and the agent
"""

import io
import json
import struct
import threading


class _PipeMessageBase(object):
    def __init__(self, fd):
        """
        `path` is the path to a local fifo pipe
        """
        self.fd = fd
        self.file = None
        self.closed = False

    def open(self):
        """
        Open the pipe for either reading or writing
        """
        raise NotImplementedError()

    def close(self):
        """
        Close the pipe when we are completely done using it
        """
        self.closed = True
        if self.file:
            self.file.close()


class PipeMessageReader(_PipeMessageBase):
    """
    A message-oriented reader from a fifo pipe.  Meant to be used as a context
    manager.
    """
    def open(self):
        self.file = io.open(self.fd, 'rb', buffering=0)

    def recv_msg(self):
        """
        Block until we receive a complete message from the pipe
        """
        print "Waiting for message"
        while True:
            size_bytes = self.file.read(4)
            if not size_bytes and not self.closed:
                self.open()
            else:
                break
        size = struct.unpack('i', size_bytes)[0]
        return self.file.read(size)


class PipeMessageWriter(_PipeMessageBase):
    """
    A message-oriented writer to a fifo pipe.  It sends length-prefixed
    messages.  The send_msg method is thread-safe.
    """
    def __init__(self, *args):
        super(PipeMessageWriter, self).__init__(*args)
        self.lock = threading.Lock()

    def open(self):
        self.file = io.open(self.fd, 'wb', buffering=0)

    def send_msg(self, type_, msg_obj):
        """
        Sends a message with the with the size prefixed to determine the
        message boundary on the receiving side.
        """
        msg_bytes = json.dumps({
            "type": type_,
            "message": msg_obj,
        })

        with self.lock:
            self.file.write(struct.pack('i', len(msg_bytes)))
            self.file.write(msg_bytes)
