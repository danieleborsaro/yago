package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// Status represents command exit status codes
type Status int

const (
	StatusOK                    Status = 0
	StatusParam                 Status = 1
	StatusTerraformError        Status = 2
	StatusMissingTool           Status = 3
	StatusDesiredStateMissing   Status = 4
	StatusDesiredStateMalformed Status = 5
	StatusConfigMissing         Status = 6
	StatusConfigMalformed       Status = 7
	StatusUndefined             Status = 255
)

// Config holds configuration for command execution
type Config struct {
	WorkingDir               string
	Env                      map[string]string // Environment variables to append
	EnvOverride              map[string]string // Environment variables to override (replaces all)
	Timeout                  time.Duration
	IsBufferedOutput         bool
	IsShowOutput             bool
	IsMeasureDuration        bool
	IsRealtimeOutput         bool
	IsRedirectStderrToStdout bool
	IsDryRun                 bool // Preview command without executing
	IsPreviewCommand         bool // Log command at info level before execution
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		WorkingDir:               ".",
		Env:                      make(map[string]string),
		EnvOverride:              make(map[string]string),
		Timeout:                  0, // No timeout
		IsBufferedOutput:         false,
		IsShowOutput:             true,
		IsMeasureDuration:        false,
		IsRealtimeOutput:         true,
		IsRedirectStderrToStdout: true,
		IsDryRun:                 false,
		IsPreviewCommand:         false,
	}
}

// Result holds the result of command execution
type Result struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  time.Duration
	StartTime time.Time
	EndTime   time.Time
}

// Shell represents a shell command executor
type Shell struct {
	config *Config
	logger *logging.Logger
}

// NewShell creates a new shell command executor
func NewShell(config *Config) *Shell {
	if config == nil {
		config = DefaultConfig()
	}

	return &Shell{
		config: config,
		logger: logging.NewLogger(logging.INFO),
	}
}

// SetLogger sets a custom logger
func (s *Shell) SetLogger(logger *logging.Logger) {
	s.logger = logger
}

// Execute runs a command and returns the result
func (s *Shell) Execute(command string, args ...string) (*Result, error) {
	return s.ExecuteWithContext(context.Background(), command, args...)
}

// ExecuteWithContext runs a command with a context and returns the result
func (s *Shell) ExecuteWithContext(ctx context.Context, command string, args ...string) (*Result, error) {
	startTime := time.Now()

	cmdStr := command
	if len(args) > 0 {
		cmdStr = fmt.Sprintf("%s %s", command, strings.Join(args, " "))
	}

	// Handle dry-run mode
	if s.config.IsDryRun {
		s.logger.Info("[Dry-Run] Command: '%s'", cmdStr)
		return &Result{
			ExitCode:  0,
			Stdout:    "",
			Stderr:    "",
			Duration:  0,
			StartTime: startTime,
			EndTime:   startTime,
		}, nil
	}

	// Handle preview mode
	if s.config.IsPreviewCommand {
		s.logger.Info("Command: '%s'", cmdStr)
	} else if s.config.IsMeasureDuration {
		s.logger.Info("Executing command: %s", cmdStr)
	} else {
		s.logger.Debug("Command: '%s'", cmdStr)
	}

	// Create context with timeout if specified
	if s.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.Timeout)
		defer cancel()
	}

	// Create the command
	cmd := exec.CommandContext(ctx, command, args...)

	// Set working directory
	if s.config.WorkingDir != "" {
		cmd.Dir = s.config.WorkingDir
	}

	// Set environment variables
	// If EnvOverride is set, use only those variables (complete replacement)
	// Otherwise, start with system environment and append Env variables
	if len(s.config.EnvOverride) > 0 {
		cmd.Env = []string{}
		for key, value := range s.config.EnvOverride {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
		s.logger.Debug("Using environment override with %d variables", len(s.config.EnvOverride))
	} else {
		cmd.Env = os.Environ()
		for key, value := range s.config.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
		if len(s.config.Env) > 0 {
			s.logger.Debug("Appending %d environment variables", len(s.config.Env))
		}
	}

	result := &Result{
		StartTime: startTime,
	}

	// Execute based on configuration
	if s.config.IsRealtimeOutput && s.config.IsShowOutput {
		err := s.executeWithRealtimeOutput(cmd, result)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)

		if s.config.IsMeasureDuration {
			s.logger.Info("Command completed in %v", result.Duration)
		}

		return result, err
	} else {
		err := s.executeBuffered(cmd, result)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)

		if s.config.IsMeasureDuration {
			s.logger.Info("Command completed in %v", result.Duration)
		}

		return result, err
	}
}

// executeWithRealtimeOutput executes command with real-time output streaming
func (s *Shell) executeWithRealtimeOutput(cmd *exec.Cmd, result *Result) error {
	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to create stdout pipe")
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to create stderr pipe")
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to start command")
	}

	// Channels for collecting output
	stdoutChan := make(chan string, 100)
	stderrChan := make(chan string, 100)
	doneChan := make(chan struct{}, 2)

	// Goroutine to read stdout
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stdoutChan <- line
			if s.config.IsShowOutput {
				fmt.Println(line)
			}
		}
		close(stdoutChan)
		doneChan <- struct{}{}
	}()

	// Goroutine to read stderr
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			stderrChan <- line
			if s.config.IsShowOutput {
				if s.config.IsRedirectStderrToStdout {
					fmt.Println(line)
				} else {
					fmt.Fprintln(os.Stderr, line)
				}
			}
		}
		close(stderrChan)
		doneChan <- struct{}{}
	}()

	// Wait for output readers to finish
	<-doneChan
	<-doneChan

	// Collect output
	var stdoutLines, stderrLines []string
	for line := range stdoutChan {
		stdoutLines = append(stdoutLines, line)
	}
	for line := range stderrChan {
		stderrLines = append(stderrLines, line)
	}

	result.Stdout = strings.Join(stdoutLines, "\n")
	result.Stderr = strings.Join(stderrLines, "\n")

	// Wait for command to complete
	err = cmd.Wait()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		} else {
			result.ExitCode = int(StatusUndefined)
		}
		return errors.Wrapf(errors.ErrFail, err, "command failed with exit code %d", result.ExitCode)
	}

	result.ExitCode = 0
	return nil
}

// executeBuffered executes command and buffers all output
func (s *Shell) executeBuffered(cmd *exec.Cmd, result *Result) error {
	var stdout, stderr strings.Builder

	cmd.Stdout = &stdout
	if s.config.IsRedirectStderrToStdout {
		cmd.Stderr = &stdout
	} else {
		cmd.Stderr = &stderr
	}

	err := cmd.Run()

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		} else {
			result.ExitCode = int(StatusUndefined)
		}

		if s.config.IsShowOutput {
			if result.Stdout != "" {
				fmt.Print(result.Stdout)
			}
			if result.Stderr != "" {
				if s.config.IsRedirectStderrToStdout {
					fmt.Print(result.Stderr)
				} else {
					fmt.Fprint(os.Stderr, result.Stderr)
				}
			}
		}

		return errors.Wrapf(errors.ErrFail, err, "command failed with exit code %d", result.ExitCode)
	}

	result.ExitCode = 0

	if s.config.IsShowOutput {
		if result.Stdout != "" {
			fmt.Print(result.Stdout)
		}
		if result.Stderr != "" {
			if s.config.IsRedirectStderrToStdout {
				fmt.Print(result.Stderr)
			} else {
				fmt.Fprint(os.Stderr, result.Stderr)
			}
		}
	}

	return nil
}

// ExecuteScript executes a shell script
func (s *Shell) ExecuteScript(script string) (*Result, error) {
	return s.ExecuteScriptWithContext(context.Background(), script)
}

// ExecuteScriptWithContext executes a shell script with context
func (s *Shell) ExecuteScriptWithContext(ctx context.Context, script string) (*Result, error) {
	// Determine shell based on OS
	shell := GetShell()
	var args []string

	if IsWindows() {
		args = []string{"-Command", script}
	} else {
		args = []string{"-c", script}
	}

	return s.ExecuteWithContext(ctx, shell, args...)
}

// Platform detection functions

// GetPlatform returns the current operating system
func GetPlatform() string {
	return runtime.GOOS
}

// IsWindows checks if running on Windows
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsLinux checks if running on Linux
func IsLinux() bool {
	return runtime.GOOS == "linux"
}

// IsDarwin checks if running on macOS
func IsDarwin() bool {
	return runtime.GOOS == "darwin"
}

// GetShell returns the appropriate shell for the current platform
func GetShell() string {
	switch runtime.GOOS {
	case "windows":
		return "powershell"
	case "linux", "darwin":
		return "/bin/bash"
	default:
		return "/bin/sh"
	}
}

// isWindows is deprecated, use IsWindows instead
// Kept for backwards compatibility
func isWindows() bool {
	return IsWindows()
}

// EscapePath escapes special characters in paths for shell execution
// Handles platform-specific escaping requirements
func EscapePath(path string) string {
	path = filepath.Clean(path)

	if IsWindows() {
		// Windows: use backtick for escaping, backslash for directory separator
		path = strings.ReplaceAll(path, "(", "`(")
		path = strings.ReplaceAll(path, ")", "`)")
		path = strings.ReplaceAll(path, " ", "` ")
		// Normalize to backslash directory separators
		path = filepath.ToSlash(path)
		path = strings.ReplaceAll(path, "/", "\\")
	} else {
		// Unix: use backslash for escaping
		path = strings.ReplaceAll(path, "(", "\\(")
		path = strings.ReplaceAll(path, ")", "\\)")
		path = strings.ReplaceAll(path, " ", "\\ ")
	}

	return path
}

// Quick utility functions for common operations

// Run executes a command with default configuration
func Run(command string, args ...string) (*Result, error) {
	shell := NewShell(DefaultConfig())
	return shell.Execute(command, args...)
}

// RunScript executes a shell script with default configuration
func RunScript(script string) (*Result, error) {
	shell := NewShell(DefaultConfig())
	return shell.ExecuteScript(script)
}

// RunSilent executes a command without showing output
func RunSilent(command string, args ...string) (*Result, error) {
	config := DefaultConfig()
	config.IsShowOutput = false
	shell := NewShell(config)
	return shell.Execute(command, args...)
}

// RunWithTimeout executes a command with a timeout
func RunWithTimeout(timeout time.Duration, command string, args ...string) (*Result, error) {
	config := DefaultConfig()
	config.Timeout = timeout
	shell := NewShell(config)
	return shell.Execute(command, args...)
}
