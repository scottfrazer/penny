package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"
)

type Logger struct {
	prefix       string
	writer       io.Writer
	mutex        *sync.Mutex
	wfLogsPath   string
	callLogsPath string
	logQueries   bool
}

func NewLogger() *Logger {
	var mutex sync.Mutex
	return &Logger{"", ioutil.Discard, &mutex, "", "", true}
}

func (log *Logger) ToFile(path string) *Logger {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(fmt.Sprintf("Failed to open log file %s: %s", path, err))
	}
	log.writer = io.MultiWriter(log.writer, file)
	return log
}

func (log *Logger) ToWriter(writer io.Writer) *Logger {
	w := io.MultiWriter(log.writer, writer)
	return &Logger{log.prefix, w, log.mutex, log.wfLogsPath, log.callLogsPath, log.logQueries}
}

func (log *Logger) Info(format string, args ...interface{}) {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	now := time.Now().Format("2006-01-02 15:04:05.999")
	fmt.Fprintf(log.writer, "INFO "+now+" "+log.prefix+fmt.Sprintf(format, args...)+"\n")
}

func (log *Logger) Debug(format string, args ...interface{}) {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	now := time.Now().Format("2006-01-02 15:04:05.999")
	fmt.Fprintf(log.writer, "DEBUG "+now+" "+log.prefix+fmt.Sprintf(format, args...)+"\n")
}

func (log *Logger) DbQuery(query string, args ...interface{}) {
	if log.logQueries {
		var message strings.Builder
		message.WriteString(fmt.Sprintf("SQL Query: %s", query))
		if len(args) > 0 {
			message.WriteString(", Args:\n")
			for index, arg := range args {
				switch v := arg.(type) {
				case int64:
					message.WriteString(fmt.Sprintf("[%d] %d\n", index, v))
				default:
					message.WriteString(fmt.Sprintf("[%d] %v\n", index, v))
				}
			}
		}
		log.Info(message.String())
	}
}
