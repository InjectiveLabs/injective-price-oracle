package oracle

import (
	"testing"
)

func TestConvertTimestampToSecond(t *testing.T) {
	tests := []struct {
		name      string
		timestamp uint64
		expected  uint64
	}{
		{
			name:      "Convert seconds to seconds",
			timestamp: 123, // seconds
			expected:  123,
		},
		{
			name:      "Convert milliseconds to seconds",
			timestamp: 1737468044594731156, // milliseconds
			expected:  1737468044,
		},
		{
			name:      "Convert microseconds to seconds",
			timestamp: 1737468044540691952, // microseconds
			expected:  1737468044,
		},
		{
			name:      "Already in seconds",
			timestamp: 1737468044994731156, // nanoseconds
			expected:  1737468044,          // 1737468044994731156 / 1000000000
		},
		{
			name:      "Edge case - maximum seconds (just below milliseconds)",
			timestamp: 999_999_999, // seconds
			expected:  999_999_999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertTimestampToSecond(tt.timestamp)
			if result != tt.expected {
				t.Errorf("ConvertTimestampToSecond(%d) = %d; want %d", tt.timestamp, result, tt.expected)
			}
		})
	}
}
