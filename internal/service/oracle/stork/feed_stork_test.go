package stork

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
			name:      "Convert microseconds to seconds",
			timestamp: 1737468044540691952, // microseconds
			expected:  1737468044,
		},
		{
			name:      "Convert nanoseconds to seconds",
			timestamp: 1738013700767706647, // nanoseconds
			expected:  1738013700,
		},
		{
			name:      "Convert nanoseconds to seconds",
			timestamp: 1738013701044503470, // nanoseconds
			expected:  1738013701,
		},
		{
			name:      "Convert nanoseconds to seconds",
			timestamp: 1738013701534503470, // nanoseconds
			expected:  1738013701,
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
