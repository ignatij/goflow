package log

import (
	"os"

	"github.com/sirupsen/logrus"
)

var logger *logrus.Logger

func init() {
	logger = logrus.New()
	level := os.Getenv("LOG_LEVEL")
	if level != "" {
		if level == "DEBUG" {
			logger.SetLevel(logrus.DebugLevel)
		} else if level == "WARN" {
			logger.SetLevel(logrus.WarnLevel)
		} else if level == "INFO" {
			logger.SetLevel(logrus.InfoLevel)
		}
	} else {
		logger.SetLevel(logrus.InfoLevel) // Default level; adjustable
	}
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	// Optional: Log to a file instead of stdout
	// file, err := os.OpenFile("goflow.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	// if err != nil {
	//     logger.Fatal("Failed to open log file: %v", err)
	// }
	// logger.SetOutput(file)
}

// GetLogger returns the shared logger instance
func GetLogger() *logrus.Logger {
	return logger
}
