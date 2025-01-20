package oracle

import (
	"testing"
)

func TestConvertTimestampToNanosecond(t *testing.T) {
	tests := []struct {
		name      string
		timestamp uint64
		expected  uint64
	}{
		{
			name:      "Convert seconds to nanoseconds",
			timestamp: 123, // seconds
			expected:  123_000_000_000,
		},
		{
			name:      "Convert milliseconds to nanoseconds",
			timestamp: 1234567890, // milliseconds
			expected:  1234567890_000_000,
		},
		{
			name:      "Convert microseconds to nanoseconds",
			timestamp: 1234567890123, // microseconds
			expected:  1234567890123_000,
		},
		{
			name:      "Already in nanoseconds",
			timestamp: 1737408937539975608, // nanoseconds
			expected:  1737408937539975608,
		},
		{
			name:      "Edge case - maximum seconds (just below milliseconds)",
			timestamp: 999_999_999, // seconds
			expected:  999_999_999_000_000_000,
		},
		{
			name:      "Edge case - maximum milliseconds (just below microseconds)",
			timestamp: 999_999_999_999, // milliseconds
			expected:  999_999_999_999_000_000,
		},
		{
			name:      "Edge case - maximum microseconds (just below nanoseconds)",
			timestamp: 999_999_999_999_999, // microseconds
			expected:  999_999_999_999_999_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertTimestampToNanosecond(tt.timestamp)
			if result != tt.expected {
				t.Errorf("convertTimestampToNanosecond(%d) = %d; want %d", tt.timestamp, result, tt.expected)
			}
		})
	}
}
