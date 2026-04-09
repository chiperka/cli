// Package result provides hierarchical test result storage with progressive disclosure.
package result

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	PrefixLocalRun  = "lr-"
	PrefixCloudRun  = "cr-"
)

// generateUUID generates a random UUID v4.
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewRunUUID generates a new local run UUID.
func NewRunUUID() string { return PrefixLocalRun + generateUUID() }

// CloudRunUUID wraps an existing cloud run ID with the cloud prefix.
func CloudRunUUID(id string) string { return PrefixCloudRun + id }

// IsLocal returns true if the UUID has a local prefix.
func IsLocal(uuid string) bool { return strings.HasPrefix(uuid, "l") }

// IsCloud returns true if the UUID has a cloud prefix.
func IsCloud(uuid string) bool { return strings.HasPrefix(uuid, "c") }
