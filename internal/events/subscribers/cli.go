package subscribers

import (
	"fmt"
	"io"
	"sync"
	"time"

	"spark-cli/internal/events"
)

// CLIReporter writes user-friendly progress output.
// Shows only completed tests with progress percentage.
// Designed for CI environments (no dynamic updates, just appending lines).
type CLIReporter struct {
	output    io.Writer
	mu        sync.Mutex
	useColor  bool
	startTime time.Time

	// Progress tracking
	totalTests int
	completed  int
	passed     int
	failed     int
	skipped    int

	// Section headers
	testHeaderShown bool
}

// NewCLIReporter creates a new CLI reporter.
func NewCLIReporter(output io.Writer) *CLIReporter {
	return &CLIReporter{
		output:   output,
		useColor: true,
	}
}

// SetColor enables or disables colored output.
func (c *CLIReporter) SetColor(enabled bool) {
	c.useColor = enabled
}

// Register subscribes this reporter to relevant events.
func (c *CLIReporter) Register(bus *events.Bus) {
	c.startTime = bus.StartTime()

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

	fmt.Fprintln(c.output)
	version, _ := e.Data.Details["version"].(string)
	if c.useColor {
		if version != "" && version != "dev" {
			fmt.Fprintf(c.output, "%sSpark Test Runner%s %sv%s%s\n", colorCyan, colorReset, colorGray, version, colorReset)
		} else {
			fmt.Fprintf(c.output, "%sSpark Test Runner%s\n", colorCyan, colorReset)
		}
	} else {
		if version != "" && version != "dev" {
			fmt.Fprintf(c.output, "Spark Test Runner v%s\n", version)
		} else {
			fmt.Fprintln(c.output, "Spark Test Runner")
		}
	}
	fmt.Fprintf(c.output, "  %d tests in %d suites", c.totalTests, suites)
	if workers, ok := e.Data.Details["workers"].(int); ok && workers > 0 {
		fmt.Fprintf(c.output, ", %d workers", workers)
	}
	fmt.Fprintln(c.output)
	fmt.Fprintln(c.output)
}

func (c *CLIReporter) onTestStarted(e *events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.showTestHeader()
}

func (c *CLIReporter) onTestCompleted(e *events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.showTestHeader()
	c.completed++
	c.passed++

	c.writeTestResult(true, e)
}

func (c *CLIReporter) onTestFailed(e *events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.showTestHeader()
	c.completed++
	c.failed++

	c.writeTestResult(false, e)
}

func (c *CLIReporter) onTestSkipped(e *events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.showTestHeader()
	c.completed++
	c.skipped++

	progress := c.testProgressPercent()

	var line string
	if c.useColor {
		line = fmt.Sprintf("  %s-%s [%3.0f%%] %s/%s %s(skipped)%s\n",
			colorYellow, colorReset,
			progress,
			e.SuiteName,
			e.TestName,
			colorGray, colorReset)
	} else {
		line = fmt.Sprintf("  - [%3.0f%%] %s/%s (skipped)\n",
			progress,
			e.SuiteName,
			e.TestName)
	}
	fmt.Fprint(c.output, line)
}

func (c *CLIReporter) writeTestResult(passed bool, e *events.Event) {
	progress := c.testProgressPercent()

	var icon, iconColor string
	if passed {
		icon = "✓"
		iconColor = colorGreen
	} else {
		icon = "✗"
		iconColor = colorRed
	}

	var line string
	if c.useColor {
		line = fmt.Sprintf("  %s%s%s [%3.0f%%] %s/%s %s(%s)%s",
			iconColor, icon, colorReset,
			progress,
			e.SuiteName,
			e.TestName,
			colorGray, e.Data.Duration.Round(time.Millisecond), colorReset)
	} else {
		line = fmt.Sprintf("  %s [%3.0f%%] %s/%s (%s)",
			icon,
			progress,
			e.SuiteName,
			e.TestName,
			e.Data.Duration.Round(time.Millisecond))
	}

	// Add error message for failures
	if !passed && e.Data.Message != "" {
		line += fmt.Sprintf(" - %s", e.Data.Message)
	}

	fmt.Fprintln(c.output, line)
}

func (c *CLIReporter) onRunCompleted(e *events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	duration := e.Data.Duration.Round(time.Millisecond)

	fmt.Fprintln(c.output)

	if c.failed > 0 {
		if c.useColor {
			fmt.Fprintf(c.output, "%sFAILED%s", colorRed, colorReset)
		} else {
			fmt.Fprint(c.output, "FAILED")
		}
		fmt.Fprintf(c.output, " %d/%d passed", c.passed, c.totalTests)
		if c.failed > 0 {
			fmt.Fprintf(c.output, " (%d failed)", c.failed)
		}
		if c.skipped > 0 {
			fmt.Fprintf(c.output, " (%d skipped)", c.skipped)
		}
	} else {
		if c.useColor {
			fmt.Fprintf(c.output, "%sPASSED%s", colorGreen, colorReset)
		} else {
			fmt.Fprint(c.output, "PASSED")
		}
		fmt.Fprintf(c.output, " %d/%d", c.passed, c.totalTests)
		if c.skipped > 0 {
			fmt.Fprintf(c.output, " (%d skipped)", c.skipped)
		}
	}

	fmt.Fprintf(c.output, " in %s\n", duration)
}

func (c *CLIReporter) testProgressPercent() float64 {
	if c.totalTests == 0 {
		return 0
	}
	return float64(c.completed) / float64(c.totalTests) * 100
}

func (c *CLIReporter) showTestHeader() {
	if c.testHeaderShown {
		return
	}
	c.testHeaderShown = true

	if c.useColor {
		fmt.Fprintf(c.output, "%sRunning tests%s\n", colorCyan, colorReset)
	} else {
		fmt.Fprintln(c.output, "Running tests")
	}
}
