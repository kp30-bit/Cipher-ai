package file

import (
	"fmt"
	"os"
	"strings"
)

// SanitizeFileName sanitizes a filename by replacing invalid characters
func SanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, ":", "-")
	return name
}

// CreateDirectory creates a directory if it doesn't exist
func CreateDirectory(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return nil
}

