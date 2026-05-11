package main

import (
	"strings"
	"testing"
)

func TestAdaptPromptForTool_NonPiUnchanged(t *testing.T) {
	in := "/belmont:implement --feature pi-smoke\n\nMILESTONE-SCOPED IMPLEMENTATION: foo."
	for _, tool := range []string{"claude", "codex", "gemini", "cursor", "copilot", ""} {
		got := adaptPromptForTool(in, tool)
		if got != in {
			t.Errorf("tool %q: expected prompt unchanged, got rewrite:\n%s", tool, got)
		}
	}
}

func TestAdaptPromptForTool_PiRewritesSlashCommand(t *testing.T) {
	in := "/belmont:implement --feature pi-smoke\n\nMILESTONE-SCOPED IMPLEMENTATION: only milestone M1."
	got := adaptPromptForTool(in, "pi")

	// The literal slash-command prefix must be gone (Pi treats it as text).
	if strings.HasPrefix(got, "/belmont:") {
		t.Errorf("rewrite still starts with literal slash command: %s", got)
	}
	// Natural-language skill reference present.
	if !strings.Contains(got, "Run the belmont:implement skill") {
		t.Errorf("expected 'Run the belmont:implement skill' phrasing, got:\n%s", got)
	}
	// Explicit SKILL.md pointer present so the model has a concrete path.
	if !strings.Contains(got, ".agents/skills/belmont/implement/SKILL.md") {
		t.Errorf("expected explicit SKILL.md path, got:\n%s", got)
	}
	// Feature name preserved.
	if !strings.Contains(got, `"pi-smoke"`) {
		t.Errorf("expected feature name in quotes, got:\n%s", got)
	}
	// MILESTONE-SCOPED suffix preserved verbatim — the rewriter must only
	// touch the slash-command line, not downstream prose.
	if !strings.Contains(got, "MILESTONE-SCOPED IMPLEMENTATION: only milestone M1.") {
		t.Errorf("expected milestone-scoped block preserved, got:\n%s", got)
	}
}

func TestAdaptPromptForTool_PiWithoutFeatureFlag(t *testing.T) {
	// belmont:tech-plan and belmont:debug-auto callers may omit --feature in
	// some code paths — the rewriter must tolerate the bare form.
	in := "/belmont:tech-plan"
	got := adaptPromptForTool(in, "pi")
	if strings.HasPrefix(got, "/belmont:") {
		t.Errorf("bare slash command still literal: %s", got)
	}
	if !strings.Contains(got, "Run the belmont:tech-plan skill") {
		t.Errorf("expected 'Run the belmont:tech-plan skill', got:\n%s", got)
	}
	if !strings.Contains(got, ".agents/skills/belmont/tech-plan/SKILL.md") {
		t.Errorf("expected SKILL.md pointer, got:\n%s", got)
	}
}

func TestAdaptPromptForTool_PiDoesNotRewriteEmbeddedReferences(t *testing.T) {
	// The rewriter is anchored to the start of the prompt (^). Mid-prompt
	// mentions of /belmont:something (e.g. in prose telling the agent about
	// other skills) must NOT be rewritten.
	in := "/belmont:implement --feature x\n\nIf you need to plan more, the user can later invoke /belmont:tech-plan."
	got := adaptPromptForTool(in, "pi")
	if !strings.Contains(got, "/belmont:tech-plan") {
		t.Errorf("mid-prompt /belmont:tech-plan reference was rewritten — should be left alone:\n%s", got)
	}
}
