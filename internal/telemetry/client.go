package telemetry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

const telemetryEndpoint = "https://spark-telemetry.finie.io/v1/events"

// send posts an event to the telemetry endpoint.
// Ignores all errors — telemetry must never affect CLI behavior.
func send(version string, event *Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", telemetryEndpoint, bytes.NewReader(data))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "spark-cli/"+version)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
