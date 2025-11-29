package app

import (
	"sort"
	"testing"
)

// Helper to sort and compare slices for test stability
func sortAndCompare(a, b []string) bool {
	// Copy slices to avoid modifying originals (if they were passed as literals, this is fine)
	// For testing, sorting the result and the expected output makes the comparison order-independent.
	res := make([]string, len(a))
	copy(res, a)
	exp := make([]string, len(b))
	copy(exp, b)

	sort.Strings(res)
	sort.Strings(exp)

	if len(res) != len(exp) {
		return false
	}
	for i := range res {
		if res[i] != exp[i] {
			return false
		}
	}
	return true
}

func TestMergeAgents(t *testing.T) {
	tests := []struct {
		name     string
		args     []any
		expected []string
	}{
		{
			name:     "Basic: Single string argument",
			args:     []any{"agentA, agentB"},
			expected: []string{"agentA", "agentB"},
		},
		{
			name:     "Basic: Single []string argument",
			args:     []any{[]string{"agentC", "agentD"}},
			expected: []string{"agentC", "agentD"},
		},
		{
			name:     "Mixed: String and []string arguments",
			args:     []any{"agentA,agentB", []string{"agentC", "agentD"}},
			expected: []string{"agentA", "agentB", "agentC", "agentD"},
		},
		{
			name:     "Deduplication: Duplicate strings",
			args:     []any{"agentA, agentB", []string{"agentA", "agentC", "agentB"}},
			expected: []string{"agentA", "agentB", "agentC"},
		},
		{
			name:     "Trimming: Handles leading/trailing spaces in string",
			args:     []any{"  agentA ,agentB, agentC   "},
			expected: []string{"agentA", "agentB", "agentC"},
		},
		{
			name:     "Trimming: Handles elements with spaces in []string",
			args:     []any{[]string{" agentD ", "agentE", "  agentF"}},
			expected: []string{"agentD", "agentE", "agentF"},
		},
		{
			name:     "Edge Case: Empty arguments",
			args:     []any{},
			expected: []string{},
		},
		{
			name:     "Edge Case: Empty string argument",
			args:     []any{""},
			expected: []string{},
		},
		{
			name:     "Edge Case: Empty []string argument",
			args:     []any{[]string{}},
			expected: []string{},
		},
		{
			name:     "Edge Case: String with only spaces and commas",
			args:     []any{" , ,   , "},
			expected: []string{},
		},
		{
			name:     "Edge Case: []string with only spaces",
			args:     []any{[]string{" ", "  ", ""}},
			expected: []string{},
		},
		{
			name:     "Edge Case: Contains nil argument",
			args:     []any{"agentA", nil, []string{"agentB"}},
			expected: []string{"agentA", "agentB"},
		},
		{
			name:     "Edge Case: Contains non-string/non-slice types (should be ignored)",
			args:     []any{"agentA", 123, true, map[string]string{}, []int{1, 2}, []string{"agentB"}},
			expected: []string{"agentA", "agentB"},
		},
		{
			name:     "Case Sensitivity: Elements are case-sensitive",
			args:     []any{"agentA", "AgentA", []string{"AGENTA"}},
			expected: []string{"agentA", "AgentA", "AGENTA"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function with variadic arguments
			var args []any
			for _, arg := range tt.args {
				args = append(args, arg)
			}
			got := mergeAgents(args...)

			if !sortAndCompare(got, tt.expected) {
				t.Errorf("mergeAgents() got = %v, want %v", got, tt.expected)
			}
		})
	}
}
