package pyrunner

import (
	"encoding/json"

	log "github.com/sirupsen/logrus"
)

// LogMessage represents the log message that comes back from python
type LogMessage struct {
	Message     string  `json:"message"`
	Level       string  `json:"level"`
	Logger      string  `json:"logger"`
	SourcePath  string  `json:"source_path"`
	LineNumber  string  `json:"lineno"`
	CreatedTime float64 `json:"created"`
}

func handleLogMessage(logMessage []byte) {
	var msg LogMessage
	err = json.Unmarshal(message, &msg)
	if err != nil {
		log.WithFields(log.Fields{
			"error":   err,
			"message": message,
		}).Error("Could not deserialize log message from Python runner")
		continue
	}

	fields := log.Fields{
		"logger":      msg.Logger,
		"sourcePath":  msg.SourcePath,
		"lineno":      msg.LineNumber,
		"createdTime": msg.CreatedTime,
	}

	switch msg.Level {
	case "DEBUG":
		log.WithFields(fields).Debug(msg.Message)
	case "INFO":
		log.WithFields(fields).Info(msg.Message)
	case "WARNING":
		log.WithFields(fields).Warn(msg.Message)
	case "ERROR":
		log.WithFields(fields).Error(msg.Message)
	case "CRITICAL":
		log.WithFields(fields).Errorf("CRITICAL: %s", msg.Message)
	default:
		log.WithFields(fields).Info(msg.Message)
	}
}
