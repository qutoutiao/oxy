package cbreaker

import (
	"testing"
	"time"

	"github.com/qutoutiao/oxy/memmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTripped(t *testing.T) {
	testCases := []struct {
		expression string
		metrics    *memmetrics.RTMetrics
		expected   bool
	}{
		{
			expression: "TotalCount() > 10",
			metrics:    statsRequestTotal(9),
			expected:   false,
		},
		{
			expression: "TotalCount() > 10",
			metrics:    statsRequestTotal(11),
			expected:   true,
		},
		{
			expression: "ResponseCodeRatio(500, 600, 0, 600) > 0.9 && TotalCount() > 10",
			metrics:    statsResponseCodes(statusCode{Code: 200, Count: 1}, statusCode{Code: 500, Count: 10}),
			expected:   true,
		},
		{
			expression: "ResponseCodeRatio(500, 600, 0, 600) > 0.9 && TotalCount() > 10",
			metrics:    statsResponseCodes(statusCode{Code: 200, Count: 1}, statusCode{Code: 500, Count: 8}),
			expected:   false,
		},
		{
			expression: "NetworkErrorRatio() > 0.5",
			metrics:    statsNetErrors(0.6),
			expected:   true,
		},
		{
			expression: "NetworkErrorRatio() < 0.5",
			metrics:    statsNetErrors(0.6),
			expected:   false,
		},
		{
			expression: "LatencyAtQuantileMS(50.0) > 50",
			metrics:    statsLatencyAtQuantile(50, time.Millisecond*51),
			expected:   true,
		},
		{
			expression: "LatencyAtQuantileMS(50.0) < 50",
			metrics:    statsLatencyAtQuantile(50, time.Millisecond*51),
			expected:   false,
		},
		{
			expression: "ResponseCodeRatio(500, 600, 0, 600) > 0.5",
			metrics:    statsResponseCodes(statusCode{Code: 200, Count: 5}, statusCode{Code: 500, Count: 6}),
			expected:   true,
		},
		{
			expression: "ResponseCodeRatio(500, 600, 0, 600) > 0.5",
			metrics:    statsResponseCodes(statusCode{Code: 200, Count: 5}, statusCode{Code: 500, Count: 4}),
			expected:   false,
		},
		{
			// quantile not defined
			expression: "LatencyAtQuantileMS(40.0) > 50",
			metrics:    statsNetErrors(0.6),
			expected:   false,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.expression, func(t *testing.T) {
			t.Parallel()

			p, err := parseExpression(test.expression)
			require.NoError(t, err)
			require.NotNil(t, p)

			assert.Equal(t, test.expected, p(&CircuitBreaker{metrics: test.metrics}))
		})
	}
}
