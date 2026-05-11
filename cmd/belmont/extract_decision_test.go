package main

import (
	"strings"
	"testing"
)

func TestExtractDecisionJSON_RawJSON(t *testing.T) {
	// Pi -p emits plain text — when the agent obediently emits ONLY the JSON
	// object and nothing else, the cheap regex path catches it.
	out := `{"action": "implement_milestone", "milestone_id": "M1", "reason": "todo"}`
	got, err := extractDecisionJSON(out, "pi")
	if err != nil {
		t.Fatalf("extractDecisionJSON: %v", err)
	}
	if !strings.Contains(got, `"action"`) || !strings.Contains(got, "implement_milestone") {
		t.Errorf("expected action JSON, got: %s", got)
	}
}

func TestExtractDecisionJSON_FencedJSON(t *testing.T) {
	// Pi often wraps JSON in markdown fences. Some models tag the fence with
	// "json", others don't. Cover both.
	cases := []struct {
		name string
		out  string
	}{
		{
			name: "tagged fence",
			out: "Here is my decision:\n\n```json\n" +
				`{"action": "verify_milestone", "milestone_id": "M2", "reason": "all tasks done"}` +
				"\n```\n",
		},
		{
			name: "untagged fence",
			out: "Some prose first.\n```\n" +
				`{"action": "skip_milestone", "milestone_id": "M3", "reason": "blocked"}` +
				"\n```\nMore prose.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractDecisionJSON(tc.out, "pi")
			if err != nil {
				t.Fatalf("extractDecisionJSON: %v", err)
			}
			if !strings.Contains(got, `"action"`) {
				t.Errorf("expected action JSON, got: %s", got)
			}
		})
	}
}

func TestExtractDecisionJSON_EmbeddedNestedJSON(t *testing.T) {
	// Brace-depth scanner must handle nested objects (the flat regex misses
	// these) and braces inside string literals.
	out := `Looking at the state, I think we should:

{
  "action": "implement_milestone",
  "milestone_id": "M5",
  "reason": "All deps verified",
  "details": {
    "estimated_tasks": 3,
    "note": "the string contains { and } which must not break depth tracking"
  }
}

Done.`
	got, err := extractDecisionJSON(out, "pi")
	if err != nil {
		t.Fatalf("extractDecisionJSON: %v", err)
	}
	// The match should include the nested object, not stop at the inner }.
	if !strings.Contains(got, `"details"`) {
		t.Errorf("expected nested object preserved, got: %s", got)
	}
	if !strings.Contains(got, `"M5"`) {
		t.Errorf("expected milestone_id preserved, got: %s", got)
	}
}

func TestExtractDecisionJSON_NoMatch(t *testing.T) {
	out := "Pi responded with prose only and no JSON at all. We should not return anything."
	if _, err := extractDecisionJSON(out, "pi"); err == nil {
		t.Errorf("expected error when no JSON present")
	}
}

func TestExtractDecisionJSON_ClaudeWrapper(t *testing.T) {
	// Existing Claude wrapper path must still work — we extended extraction,
	// not replaced it.
	out := `{"result": "{\"action\": \"verify_milestone\", \"milestone_id\": \"M1\", \"reason\": \"ok\"}"}`
	got, err := extractDecisionJSON(out, "claude")
	if err != nil {
		t.Fatalf("extractDecisionJSON: %v", err)
	}
	if !strings.Contains(got, "verify_milestone") {
		t.Errorf("expected unwrapped action JSON, got: %s", got)
	}
}
