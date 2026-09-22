package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// LogLevel represents different log levels
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String returns the string representation of a log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger represents a custom logger with colored output
type Logger struct {
	level        LogLevel
	output       io.Writer
	prefix       string
	showCaller   bool
	commandStart time.Time
}

// NewLogger creates a new logger instance
func NewLogger(level LogLevel) *Logger {
	return &Logger{
		level:      level,
		output:     os.Stderr,
		prefix:     "",
		showCaller: true,
	}
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// SetOutput sets the output writer
func (l *Logger) SetOutput(w io.Writer) {
	l.output = w
}

// SetPrefix sets a prefix for all log messages
func (l *Logger) SetPrefix(prefix string) {
	l.prefix = prefix
}

// SetShowCaller enables or disables caller information
func (l *Logger) SetShowCaller(show bool) {
	l.showCaller = show
}

// StartCommandTimer starts the timer for command execution time tracking
func (l *Logger) StartCommandTimer() {
	l.commandStart = time.Now()
}

// PrintExecutionTime prints the total execution time for the command
func (l *Logger) PrintExecutionTime() {
	if !l.commandStart.IsZero() {
		elapsed := time.Since(l.commandStart)
		durationStr := color.New(color.FgCyan, color.Bold).Sprintf("%.3fs", elapsed.Seconds())
		fmt.Fprintf(l.output, "\n⏱  Execution time: %s\n", durationStr)
	}
}

// log is the main logging method
func (l *Logger) log(level LogLevel, skip int, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	// Get caller information
	var caller string
	if l.showCaller {
		_, file, line, ok := runtime.Caller(skip)
		if ok {
			// Extract just the filename from the full path
			// Use LastIndex instead of Split for better performance
			lastSlash := strings.LastIndex(file, "/")
			filename := file
			if lastSlash >= 0 {
				filename = file[lastSlash+1:]
			}
			caller = fmt.Sprintf("[%s:%d]", filename, line)
		}
	}

	// Format the message
	message := fmt.Sprintf(format, args...)

	// Apply colors based on level
	// Using Solarized-compatible colors:
	// - Green for INFO (readable in both light/dark Solarized)
	// - Yellow for WARN (standard warning color)
	// - Red for ERROR (standard error color)
	// - Magenta for DEBUG (works well in Solarized)
	// - Bold Red for FATAL (critical emphasis)
	var levelStr string
	switch level {
	case DEBUG:
		levelStr = color.New(color.FgMagenta).Sprintf("[DEBUG]")
	case INFO:
		levelStr = color.New(color.FgGreen, color.Bold).Sprintf("[INFO]")
	case WARN:
		levelStr = color.New(color.FgYellow).Sprintf("[WARN]")
	case ERROR:
		levelStr = color.New(color.FgRed).Sprintf("[ERROR]")
	case FATAL:
		levelStr = color.New(color.FgRed, color.Bold).Sprintf("[FAIL]")
	}

	// Construct the full log message
	// Format: [LOGLEVEL][modulefile:line] Message
	// Timestamp only for DEBUG level
	var fullMessage string
	if level == DEBUG {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		if l.showCaller && caller != "" {
			fullMessage = fmt.Sprintf("%s %s%s %s\n", timestamp, levelStr, caller, message)
		} else {
			fullMessage = fmt.Sprintf("%s %s %s\n", timestamp, levelStr, message)
		}
	} else {
		if l.showCaller && caller != "" {
			fullMessage = fmt.Sprintf("%s%s %s\n", levelStr, caller, message)
		} else {
			fullMessage = fmt.Sprintf("%s %s\n", levelStr, message)
		}
	}

	// Write to output
	fmt.Fprint(l.output, fullMessage)

	// Exit on fatal
	if level == FATAL {
		os.Exit(1)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level > DEBUG {
		return
	}
	l.log(DEBUG, 2, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level > INFO {
		return
	}
	l.log(INFO, 2, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, 2, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, 2, format, args...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, 2, format, args...)
}

// Global logger instance
var globalLogger = NewLogger(INFO)

// Global logging functions for convenience
func SetLevel(level LogLevel) {
	globalLogger.SetLevel(level)
}

func SetOutput(w io.Writer) {
	globalLogger.SetOutput(w)
}

func SetPrefix(prefix string) {
	globalLogger.SetPrefix(prefix)
}

func SetShowCaller(show bool) {
	globalLogger.SetShowCaller(show)
}

func StartCommandTimer() {
	globalLogger.StartCommandTimer()
}

func PrintExecutionTime() {
	globalLogger.PrintExecutionTime()
}

func Debug(format string, args ...interface{}) {
	if globalLogger.level > DEBUG {
		return
	}
	globalLogger.log(DEBUG, 2, format, args...)
}

func Info(format string, args ...interface{}) {
	if globalLogger.level > INFO {
		return
	}
	globalLogger.log(INFO, 2, format, args...)
}

func Warn(format string, args ...interface{}) {
	globalLogger.log(WARN, 2, format, args...)
}

func Error(format string, args ...interface{}) {
	globalLogger.log(ERROR, 2, format, args...)
}

func Fatal(format string, args ...interface{}) {
	globalLogger.log(FATAL, 2, format, args...)
}

// Spaces prints a blank line
func Spaces() {
	fmt.Fprintln(globalLogger.output)
}

// ParseLevel parses a string log level
func ParseLevel(levelStr string) (LogLevel, error) {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return DEBUG, nil
	case "INFO":
		return INFO, nil
	case "WARN", "WARNING":
		return WARN, nil
	case "ERROR":
		return ERROR, nil
	case "FATAL":
		return FATAL, nil
	default:
		return INFO, errors.Newf(errors.ErrParam, "unknown log level: %s", levelStr)
	}
}

// StandardLogger returns a standard library logger that uses our custom logger
func StandardLogger() *log.Logger {
	return log.New(&logWriter{globalLogger}, "", 0)
}

// logWriter adapts our Logger to io.Writer interface for standard library compatibility
type logWriter struct {
	logger *Logger
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	lw.logger.log(INFO, 4, "%s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
