// Copyright 2018 MOBIKE, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package agent

import (
	"bytes"
	"fmt"
	"strings"

	"os"

	"io"

	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultLogTimeFormat = "2006/01/02 15:04:05.000"
	defaultLogMaxSize    = 300 // MB
	defaultLogFormat     = "text"
	defaultLogLevel      = log.InfoLevel
)

// textFormatter is customized text formatter.
type textFormatter struct {
	DisableTimestamp bool
}

// Format implements logrus.Formatter.
func (f *textFormatter) Format(entry *log.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}
	if !f.DisableTimestamp {
		fmt.Fprintf(b, "%s ", entry.Time.Format(defaultLogTimeFormat))
	}
	if file, ok := entry.Data["file"]; ok {
		fmt.Fprintf(b, "%s:%v ", file, entry.Data["line"])
	}
	fmt.Fprintf(b, "[%s] %s", entry.Level.String(), entry.Message)
	for k, v := range entry.Data {
		if k != "file" && k != "line" {
			fmt.Fprintf(b, " %v=%v", k, v)
		}
	}
	b.WriteByte('\n')
	return b.Bytes(), nil
}

// WarnHook is used to write log whose level higher than and equal to warn to another file
type WarnHook struct {
	formatter log.Formatter
	out       io.Writer
}

// Levels return the log level that fires the "Fire" function
func (h WarnHook) Levels() []log.Level {
	return []log.Level{
		log.WarnLevel,
		log.ErrorLevel,
		log.FatalLevel,
		log.PanicLevel,
	}
}

// Fire is used to write entry to log file
func (h WarnHook) Fire(e *log.Entry) error {
	line, err := h.formatter.Format(e)
	if err == nil {
		h.out.Write(line)
	}
	return err
}

func initWarnHook(formatter log.Formatter, output io.Writer) log.Hook {
	return WarnHook{
		formatter: formatter,
		out:       output,
	}
}

// InitLogger initializes Agent's logger.
func InitLogger(cfg *Config) error {
	log.SetLevel(stringToLogLevel(cfg.LogLevel))
	formatter := stringToLogFormatter(defaultLogFormat, false)
	log.SetFormatter(formatter)
	var output io.Writer
	if cfg.LogFile != "" {
		o, err := initFileLog(cfg.LogFile, cfg)
		if err != nil {
			return errors.Trace(err)
		}
		output = o
	} else {
		output = os.Stdout
	}
	log.SetOutput(output)

	if cfg.ErrorLog != "" {
		warnOutput, err := initFileLog(cfg.ErrorLog, cfg)
		if err == nil {
			warnHook := initWarnHook(formatter, warnOutput)
			log.AddHook(warnHook)
		}
	}

	return nil
}

// initFileLog initializes file based logging options.
func initFileLog(filename string, cfg *Config) (io.Writer, error) {
	if st, err := os.Stat(filename); err == nil {
		if st.IsDir() {
			return nil, errors.New("can't use directory as log file name")
		}
	}
	if cfg.LogMaxSize == 0 {
		cfg.LogMaxSize = defaultLogMaxSize
	}

	// use lumberjack to logrotate
	output := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    cfg.LogMaxSize,
		MaxBackups: 30,
		MaxAge:     cfg.LogMaxDays,
		LocalTime:  true,
	}
	return output, nil
}

// stringToLogFormatter returns log formatter.
func stringToLogFormatter(format string, disableTimestamp bool) log.Formatter {
	switch strings.ToLower(format) {
	case "text":
		return &textFormatter{
			DisableTimestamp: disableTimestamp,
		}
	case "json":
		return &log.JSONFormatter{
			TimestampFormat:  defaultLogTimeFormat,
			DisableTimestamp: disableTimestamp,
		}
	case "console":
		return &log.TextFormatter{
			FullTimestamp:    true,
			TimestampFormat:  defaultLogTimeFormat,
			DisableTimestamp: disableTimestamp,
		}
	default:
		return &textFormatter{}
	}
}

func stringToLogLevel(level string) log.Level {
	switch strings.ToLower(level) {
	case "fatal":
		return log.FatalLevel
	case "error":
		return log.ErrorLevel
	case "warn", "warning":
		return log.WarnLevel
	case "debug":
		return log.DebugLevel
	case "info":
		return log.InfoLevel
	}
	return defaultLogLevel
}
