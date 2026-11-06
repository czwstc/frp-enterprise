// Copyright 2016 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log

import (
	"bytes"
	"io"
	"os"
	"time"

	"github.com/fatedier/golib/log"
)

var (
	TraceLevel = log.TraceLevel
	DebugLevel = log.DebugLevel
	InfoLevel  = log.InfoLevel
	WarnLevel  = log.WarnLevel
	ErrorLevel = log.ErrorLevel
)

var Logger *log.Logger

func init() {
	Logger = log.New(
		log.WithCaller(true),
		log.AddCallerSkip(1),
		log.WithLevel(log.InfoLevel),
	)
}

// memory, when set, receives a copy of every log line, for the dashboard.
var memory *LogBuffer

// EnableMemory keeps the latest capacity log lines in memory from the next
// InitLogger on, and returns the buffer.
func EnableMemory(capacity int) *LogBuffer {
	memory = NewLogBuffer(capacity, nil)
	return memory
}

// Memory returns the in-memory log buffer, or nil when it is not enabled.
func Memory() *LogBuffer { return memory }

func InitLogger(logPath string, levelStr string, maxDays int, disableLogColor bool) {
	var out io.Writer = log.DefaultWriter
	if logPath == "console" {
		if !disableLogColor {
			out = log.NewConsoleWriter(log.ConsoleConfig{
				Colorful: true,
			}, os.Stdout)
		}
	} else {
		writer := log.NewRotateFileWriter(log.RotateFileConfig{
			FileName: logPath,
			Mode:     log.RotateFileModeDaily,
			MaxDays:  maxDays,
		})
		writer.Init()
		out = writer
	}
	if memory != nil {
		out = &teeWriter{outs: []io.Writer{out, memory}}
	}

	level, err := log.ParseLevel(levelStr)
	if err != nil {
		level = log.InfoLevel
	}
	Logger = Logger.WithOptions(log.WithOutput(out), log.WithLevel(level))
}

// teeWriter writes each line to several outputs, passing the level to those
// that take it.
type teeWriter struct {
	outs []io.Writer
}

func (t *teeWriter) Write(p []byte) (int, error) {
	for _, o := range t.outs {
		_, _ = o.Write(p)
	}
	return len(p), nil
}

func (t *teeWriter) WriteLog(p []byte, level log.Level, when time.Time) (int, error) {
	for _, o := range t.outs {
		if lw, ok := o.(log.Writer); ok {
			_, _ = lw.WriteLog(p, level, when)
		} else {
			_, _ = o.Write(p)
		}
	}
	return len(p), nil
}

// Level is a log level.
type Level = log.Level

// LevelWriter is an output that also receives the level and time of each
// line. Outputs given to InitLoggerWithWriter may implement it.
type LevelWriter = log.Writer

// InitLoggerWithWriter sends all log output to w, for programs that embed
// frp and show its logs themselves.
func InitLoggerWithWriter(w io.Writer, levelStr string) {
	level, err := log.ParseLevel(levelStr)
	if err != nil {
		level = log.InfoLevel
	}
	Logger = Logger.WithOptions(log.WithOutput(w), log.WithLevel(level))
}

func Errorf(format string, v ...any) {
	Logger.Errorf(format, v...)
}

func Warnf(format string, v ...any) {
	Logger.Warnf(format, v...)
}

func Infof(format string, v ...any) {
	Logger.Infof(format, v...)
}

func Debugf(format string, v ...any) {
	Logger.Debugf(format, v...)
}

func Tracef(format string, v ...any) {
	Logger.Tracef(format, v...)
}

func Logf(level log.Level, offset int, format string, v ...any) {
	Logger.Logf(level, offset, format, v...)
}

type WriteLogger struct {
	level  log.Level
	offset int
}

func NewWriteLogger(level log.Level, offset int) *WriteLogger {
	return &WriteLogger{
		level:  level,
		offset: offset,
	}
}

func (w *WriteLogger) Write(p []byte) (n int, err error) {
	Logger.Log(w.level, w.offset, string(bytes.TrimRight(p, "\n")))
	return len(p), nil
}
