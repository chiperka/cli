package subscribers

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"spark-cli/internal/events"
)

// DebugLogger writes detailed docker command information.
// This is for debugging mode - shows every docker command executed.
type DebugLogger struct {
	output    io.Writer
	mu        sync.Mutex
	useColor  bool
	startTime time.Time
}

// NewDebugLogger creates a new debug logger.
func NewDebugLogger(output io.Writer) *DebugLogger {
	return &DebugLogger{
		output:   output,
		useColor: true,
	}
}

// SetColor enables or disables colored output.
func (d *DebugLogger) SetColor(enabled bool) {
	d.useColor = enabled
}

// Register subscribes this logger to docker-related events.
func (d *DebugLogger) Register(bus *events.Bus) {
	d.startTime = bus.StartTime()

	// Subscribe to docker command events
	bus.OnMany(d.handle,
		events.DockerCommand,
		events.DockerCommandDone,
		events.ContainerStarted,
		events.ContainerStopped,
		events.ContainerLogs,
		events.NetworkCreated,
		events.NetworkRemoved,
	)
}

func (d *DebugLogger) handle(e *events.Event) {
	d.mu.Lock()
	defer d.mu.Unlock()

	elapsed := e.Timestamp.Sub(d.startTime)

	// Format based on event type
	switch e.Type {
	case events.DockerCommand:
		d.writeDockerCommand(elapsed, e)
	case events.DockerCommandDone:
		d.writeDockerCommandDone(elapsed, e)
	default:
		d.writeGenericDocker(elapsed, e)
	}
}

func (d *DebugLogger) writeDockerCommand(elapsed time.Duration, e *events.Event) {
	cmd, _ := e.Data.Details["command"].(string)
	args, _ := e.Data.Details["args"].([]string)

	var line string
	if d.useColor {
		line = fmt.Sprintf("%s[%7.3fs]%s %s$%s %s %s\n",
			colorGray, elapsed.Seconds(), colorReset,
			colorCyan, colorReset,
			cmd, strings.Join(args, " "))
	} else {
		line = fmt.Sprintf("[%7.3fs] $ %s %s\n",
			elapsed.Seconds(),
			cmd, strings.Join(args, " "))
	}
	fmt.Fprint(d.output, line)
}

func (d *DebugLogger) writeDockerCommandDone(elapsed time.Duration, e *events.Event) {
	duration := e.Data.Duration

	var status string
	if e.Data.Error != nil {
		if d.useColor {
			status = fmt.Sprintf("%sFAILED%s %v", colorRed, colorReset, e.Data.Error)
		} else {
			status = fmt.Sprintf("FAILED %v", e.Data.Error)
		}
	} else {
		if d.useColor {
			status = fmt.Sprintf("%sOK%s", colorGreen, colorReset)
		} else {
			status = "OK"
		}
	}

	var line string
	if d.useColor {
		line = fmt.Sprintf("%s[%7.3fs]%s   └─ %s (%s)\n",
			colorGray, elapsed.Seconds(), colorReset,
			status, duration.Round(time.Millisecond))
	} else {
		line = fmt.Sprintf("[%7.3fs]   └─ %s (%s)\n",
			elapsed.Seconds(),
			status, duration.Round(time.Millisecond))
	}
	fmt.Fprint(d.output, line)
}

func (d *DebugLogger) writeGenericDocker(elapsed time.Duration, e *events.Event) {
	var icon string
	var details string

	switch e.Type {
	case events.ContainerStarted:
		icon = "+"
		if img, ok := e.Data.Details["image"].(string); ok {
			details = fmt.Sprintf("container %s", img)
		} else {
			details = "container started"
		}
	case events.ContainerStopped:
		icon = "-"
		details = "container stopped"
	case events.NetworkCreated:
		icon = "+"
		if net, ok := e.Data.Details["network"].(string); ok {
			details = fmt.Sprintf("network %s", net)
		} else {
			details = "network created"
		}
	case events.NetworkRemoved:
		icon = "-"
		details = "network removed"
	default:
		icon = "*"
		details = string(e.Type)
	}

	var line string
	if d.useColor {
		line = fmt.Sprintf("%s[%7.3fs]%s   %s%s%s %s\n",
			colorGray, elapsed.Seconds(), colorReset,
			colorYellow, icon, colorReset,
			details)
	} else {
		line = fmt.Sprintf("[%7.3fs]   %s %s\n",
			elapsed.Seconds(), icon, details)
	}
	fmt.Fprint(d.output, line)
}
