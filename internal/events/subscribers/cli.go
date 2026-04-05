package subscribers

import (
	"fmt"
	"io"
	"sync"
	"time"

	"chiperka-cli/internal/events"
)

// CLIReporter writes user-friendly progress output.
// Shows only completed tests with progress percentage.
// Designed for CI environments (no dynamic updates, just appending lines).
type CLIReporter struct {
	out       io.Writer
	color     bool
	cloudMode bool
	mu        sync.Mutex

	// Progress tracking
	totalTests  int
	completed   int
	passed      int
	failed      int
	skipped     int
	headerShown bool
}

// NewCLIReporter creates a new CLI reporter.
func NewCLIReporter(w io.Writer) *CLIReporter {
	return &CLIReporter{
		out:   w,
		color: true,
	}
}

// SetColor enables or disables colored output.
func (c *CLIReporter) SetColor(enabled bool) {
	c.color = enabled
}

// SetCloudMode sets the cloud mode flag for display purposes.
func (c *CLIReporter) SetCloudMode(enabled bool) {
	c.cloudMode = enabled
}

// Register subscribes this reporter to relevant events.
func (c *CLIReporter) Register(bus *events.Bus) {
	bus.On(events.RunStarted, c.onRunStarted)
	bus.On(events.TestStarted, c.onTestStarted)
	bus.On(events.TestCompleted, c.onTestCompleted)
	bus.On(events.TestFailed, c.onTestFailed)
	bus.On(events.TestSkipped, c.onTestSkipped)
	bus.On(events.RunCompleted, c.onRunCompleted)
}

func (c *CLIReporter) onRunStarted(e *events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if tests, ok := e.Data.Details["tests"].(int); ok {
		c.totalTests = tests
	}
	suites, _ := e.Data.Details["suites"].(int)

	fmt.Fprintln(c.out)
	version, _ := e.Data.Details["version"].(string)
	if c.color {
		if version != "" && version != "dev" {
			fmt.Fprintf(c.out, "%sChiperka Test Runner%s %sv%s%s\n", colorCyan, colorReset, colorGray, version, colorReset)
		} else {
			fmt.Fprintf(c.out, "%sChiperka Test Runner%s\n", colorCyan, colorReset)
		}
	} else {
		if version != "" && version != "dev" {
			fmt.Fprintf(c.out, "Chiperka Test Runner v%s\n", version)
		} else {
			fmt.Fprintln(c.out, "Chiperka Test Runner")
		}
	}
	fmt.Fprintf(c.out, "  %d tests in %d suites", c.totalTests, suites)
	if workers, ok := e.Data.Details["workers"].(int); ok && workers > 0 {
		fmt.Fprintf(c.out, ", %d workers", workers)
	}
	fmt.Fprintln(c.out)
	fmt.Fprintln(c.out)
}

func (c *CLIReporter) onTestStarted(e *events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.headerShown {
		return
	}
	c.headerShown = true

	header := "Running tests"
	if c.cloudMode {
		header = "Running tests in Cloud"
	}
	if c.color {
		fmt.Fprintf(c.out, "%s%s%s\n", colorCyan, header, colorReset)
	} else {
		fmt.Fprintln(c.out, header)
	}
}

func (c *CLIReporter) onTestCompleted(e *events.Event) {
	c.mu.Lock()
	c.completed++
	c.passed++
	progress := c.testProgressPercent()
	c.mu.Unlock()

	c.writeTestResult("passed", progress, e.SuiteName+"/"+e.TestName, e.Data.Duration)
}

func (c *CLIReporter) onTestFailed(e *events.Event) {
	c.mu.Lock()
	c.completed++
	c.failed++
	progress := c.testProgressPercent()
	c.mu.Unlock()

	c.writeTestResult("failed", progress, e.SuiteName+"/"+e.TestName, e.Data.Duration)
}

func (c *CLIReporter) onTestSkipped(e *events.Event) {
	c.mu.Lock()
	c.completed++
	c.skipped++
	progress := c.testProgressPercent()
	c.mu.Unlock()

	c.writeTestResult("skipped", progress, e.SuiteName+"/"+e.TestName, 0)
}

func (c *CLIReporter) onRunCompleted(e *events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Prefer server-side counts from run_completed event (authoritative),
	// fall back to locally tracked counts if not available (standalone mode).
	passed := c.passed
	failed := c.failed
	skipped := c.skipped
	if p, ok := e.Data.Details["passed"].(int); ok {
		passed = p
	}
	if f, ok := e.Data.Details["failed"].(int); ok {
		failed = f
	}
	if s, ok := e.Data.Details["skipped"].(int); ok {
		skipped = s
	}

	c.writeSummary(passed, failed, skipped, e.Data.Duration)
}

func (c *CLIReporter) testProgressPercent() float64 {
	if c.totalTests == 0 {
		return 0
	}
	return float64(c.completed) / float64(c.totalTests) * 100
}

func (c *CLIReporter) writeTestResult(status string, progressPct float64, name string, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	icon, iconColor := statusIcon(status)
	durStr := fmt.Sprintf("(%s)", duration.Round(time.Millisecond))

	if c.color {
		fmt.Fprintf(c.out, "  %s%s%s [%3.0f%%] %s %s%s%s\n",
			iconColor, icon, colorReset,
			progressPct,
			name,
			colorGray, durStr, colorReset)
	} else {
		fmt.Fprintf(c.out, "  %s [%3.0f%%] %s %s\n",
			icon,
			progressPct,
			name,
			durStr)
	}
}

func (c *CLIReporter) writeSummary(passed, failed, skipped int, duration time.Duration) {
	total := passed + failed + skipped
	fmt.Fprintln(c.out)

	if failed > 0 {
		if c.color {
			fmt.Fprintf(c.out, "%sFAILED%s", colorRed, colorReset)
		} else {
			fmt.Fprint(c.out, "FAILED")
		}
		fmt.Fprintf(c.out, " %d/%d (%d failed)", passed, total, failed)
	} else {
		if c.color {
			fmt.Fprintf(c.out, "%sPASSED%s", colorGreen, colorReset)
		} else {
			fmt.Fprint(c.out, "PASSED")
		}
		fmt.Fprintf(c.out, " %d/%d", passed, total)
	}

	if skipped > 0 {
		fmt.Fprintf(c.out, " (%d skipped)", skipped)
	}

	fmt.Fprintf(c.out, " in %s\n", duration.Round(time.Millisecond))
}

func statusIcon(status string) (icon string, color string) {
	switch status {
	case "passed":
		return "✓", colorGreen
	case "failed", "error":
		return "✗", colorRed
	case "skipped":
		return "-", colorYellow
	default:
		return "?", colorGray
	}
}
