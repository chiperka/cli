// Package subscribers provides event handlers for different output modes.
package subscribers

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"chiperka-cli/internal/events"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// VerboseLogger writes all events in logfmt format.
// This is for detailed logging (container stdout, log files).
type VerboseLogger struct {
	output    io.Writer
	mu        sync.Mutex
	useColor  bool
	startTime time.Time
}

// NewVerboseLogger creates a new verbose logger.
func NewVerboseLogger(output io.Writer) *VerboseLogger {
	return &VerboseLogger{
		output:    output,
		useColor:  true,
		startTime: time.Now(),
	}
}

// SetColor enables or disables colored output.
func (v *VerboseLogger) SetColor(enabled bool) {
	v.useColor = enabled
}

// Register subscribes this logger to receive all events.
func (v *VerboseLogger) Register(bus *events.Bus) {
	v.startTime = bus.StartTime()
	bus.OnAll(v.handle)
}

func (v *VerboseLogger) handle(e *events.Event) {
	v.mu.Lock()
	defer v.mu.Unlock()

	elapsed := e.Timestamp.Sub(v.startTime)

	var parts []string

	// Time
	timeStr := fmt.Sprintf("time=%.3fs", elapsed.Seconds())
	if v.useColor {
		parts = append(parts, fmt.Sprintf("%s%s%s", colorGray, timeStr, colorReset))
	} else {
		parts = append(parts, timeStr)
	}

	// Level based on event type
	level := v.levelForEvent(e.Type)
	levelStr := fmt.Sprintf("level=%s", level)
	if v.useColor {
		parts = append(parts, fmt.Sprintf("%s%s%s", v.colorForLevel(level), levelStr, colorReset))
	} else {
		parts = append(parts, levelStr)
	}

	// For log events, show action instead of event type
	if v.isLogEvent(e.Type) {
		if action, ok := e.Data.Details["action"].(string); ok {
			parts = append(parts, fmt.Sprintf("action=%s", action))
		}
	} else {
		// Event type for lifecycle events
		parts = append(parts, fmt.Sprintf("event=%s", e.Type))
	}

	// Context
	if e.SuiteName != "" {
		parts = append(parts, fmt.Sprintf("suite=%s", quoteIfNeeded(e.SuiteName)))
	}
	if e.TestName != "" {
		parts = append(parts, fmt.Sprintf("test=%s", quoteIfNeeded(e.TestName)))
	}

	// Event data
	if e.Data.Status != "" {
		parts = append(parts, fmt.Sprintf("status=%s", e.Data.Status))
	}
	if e.Data.Duration > 0 {
		parts = append(parts, fmt.Sprintf("duration=%s", e.Data.Duration.Round(time.Millisecond)))
	}
	if e.Data.Current > 0 || e.Data.Total > 0 {
		parts = append(parts, fmt.Sprintf("progress=%d/%d", e.Data.Current, e.Data.Total))
	}

	// Details - show all for log events, selected for lifecycle events
	if v.isLogEvent(e.Type) {
		// Show all details except action (already shown) and msg (shown last)
		for k, val := range e.Data.Details {
			if k == "action" || k == "msg" {
				continue
			}
			if s, ok := val.(string); ok {
				parts = append(parts, fmt.Sprintf("%s=%s", k, quoteIfNeeded(s)))
			}
		}
	} else {
		// Selected details for lifecycle events
		if svc, ok := e.Data.Details["service"].(string); ok {
			parts = append(parts, fmt.Sprintf("service=%s", quoteIfNeeded(svc)))
		}
		if img, ok := e.Data.Details["image"].(string); ok {
			parts = append(parts, fmt.Sprintf("image=%s", quoteIfNeeded(img)))
		}
		if net, ok := e.Data.Details["network"].(string); ok {
			parts = append(parts, fmt.Sprintf("network=%s", quoteIfNeeded(net)))
		}
	}

	// Message last
	if e.Data.Message != "" {
		parts = append(parts, fmt.Sprintf("msg=%s", quoteIfNeeded(e.Data.Message)))
	}
	if e.Data.Error != nil {
		parts = append(parts, fmt.Sprintf("error=%s", quoteIfNeeded(e.Data.Error.Error())))
	}

	fmt.Fprintln(v.output, strings.Join(parts, " "))
}

func (v *VerboseLogger) isLogEvent(t events.Type) bool {
	return t == events.Log || t == events.LogInfo || t == events.LogPass || t == events.LogFail || t == events.LogWarn
}

func (v *VerboseLogger) levelForEvent(t events.Type) string {
	switch t {
	case events.TestCompleted, events.RunCompleted, events.LogPass:
		return "pass"
	case events.TestFailed, events.LogFail:
		return "fail"
	case events.TestSkipped, events.LogWarn:
		return "warn"
	default:
		return "info"
	}
}

func (v *VerboseLogger) colorForLevel(level string) string {
	switch level {
	case "pass":
		return colorGreen
	case "fail":
		return colorRed
	case "skip":
		return colorYellow
	default:
		return colorBlue
	}
}

func quoteIfNeeded(s string) string {
	if strings.ContainsAny(s, " \t\n\"") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
