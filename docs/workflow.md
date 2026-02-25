# Full Workflow

A step-by-step walkthrough of a complete Belmont session, from product vision to iteration.

## 0. Define Product Vision (optional)

If you're building a product with multiple features, start with a PR/FAQ to define the strategic vision.

```
Claude Code:  /belmont:working-backwards
Cursor:       Enable the belmont working-backwards rule, then: "Let's define the product vision"
Other:        Load skills/belmont/working-backwards.md as context
```

**What happens:**
- You describe the product idea and target customer
- AI asks focused questions about the problem, solution, and key benefit
- AI writes a PR/FAQ: press release + customer/stakeholder FAQs + appendix
- AI writes `.belmont/PR_FAQ.md`

## 1. Install

```bash
cd ~/projects/my-app
belmont install
```

## 2. Plan

Start an interactive planning session. Describe what you want to build. The AI will ask clarifying questions, then write a structured PRD with prioritized tasks organized into milestones.

```
Claude Code:  /belmont:product-plan
Cursor:       Enable the belmont product-plan rule, then: "Let's plan a new feature"
Other:        Load skills/belmont/product-plan.md as context
```

**What happens:**
- You describe the feature
- AI asks questions one at a time (edge cases, dependencies, Figma URLs, etc.)
- You finalize the plan together
- AI writes `.belmont/PRD.md` and `.belmont/PROGRESS.md`

It is strongly recommended you read the PRD created yourself. You can manually make edits before tech plan/implementation or you can run `belmont:product-plan` again and tell it what to refine.

## 3. Tech Plan (recommended)

Have a senior architect agent review the PRD and produce a detailed technical plan. This step is optional but strongly recommended -- it produces the TECH_PLAN.md that guides the implementation agents.

You may add any additional context to the tech plan agent that you want to include.

```
Claude Code:  /belmont:tech-plan
Cursor:       Enable the belmont tech-plan rule, then: "Let's review the technical plan"
Other:        Load skills/belmont/tech-plan.md as context
```

**What happens:**
- AI reads the PRD and explores the codebase
- Interactive discussion about architecture, patterns, edge cases
- AI writes `.belmont/TECH_PLAN.md` with file structures, component specs, API types

## 4. Implement

Run the implementation pipeline. The AI finds the next incomplete milestone and works through each task using the 4-phase agent pipeline.

```
Claude Code:  /belmont:implement
Cursor:       Enable the belmont implement rule, then: "Implement the next milestone"
Other:        Load skills/belmont/implement.md as context
```

**What happens:**
1. Orchestrator creates `.belmont/MILESTONE.md` with task list, PRD context, and TECH_PLAN context
2. `codebase-agent` reads MILESTONE, scans codebase, writes patterns to MILESTONE *(parallel with 3)*
3. `design-agent` reads MILESTONE, loads Figma, writes design specs to MILESTONE *(parallel with 2)*
4. `implementation-agent` reads MILESTONE (only), writes code, tests, verification, commits
5. PRD.md and PROGRESS.md are updated, follow-up tasks created
6. MILESTONE file is archived (`MILESTONE-M2.done.md`)

**After all tasks in the milestone:**
- Milestone is marked complete in PROGRESS.md
- MILESTONE file is archived
- Summary is reported

## 5. Quick Fix (optional)

If verification created follow-up tasks or there's a small task to knock out, use `next` to implement just one task without the full pipeline overhead.

```
Claude Code:  /belmont:next
Cursor:       Enable the belmont next rule, then: "Implement the next task"
Other:        Load skills/belmont/next.md as context
```

**What happens:**
- Finds the next unchecked task in the current milestone
- Creates a minimal MILESTONE file with the task's context (skips analysis sub-agents)
- Dispatches the single task to the implementation agent
- Task is implemented, verified, committed, and marked complete
- MILESTONE file is archived
- Reports a brief summary

## 6. Verify

Run comprehensive verification on all completed work.

```
Claude Code:  /belmont:verify
Cursor:       Enable the belmont verify rule, then: "Verify the completed tasks"
Other:        Load skills/belmont/verify.md as context
```

**What happens:**
- Verification agent checks acceptance criteria, visual fidelity, i18n
- Code review agent runs build, tests, reviews code quality
- Issues become follow-up tasks in the PRD
- Combined report is produced

## 7. Review Alignment (recommended periodically)

After implementing milestones or making significant changes, review the alignment between your plans and the codebase.

```
Claude Code:  /belmont:review
Cursor:       Enable the belmont review rule, then: "Review document alignment"
Other:        Load skills/belmont/review.md as context
```

**What happens:**
- Reads all planning documents (PR/FAQ, master PRD, feature PRDs, tech plans, PROGRESS files)
- Scans codebase for implemented features and compares against plans
- Presents each discrepancy interactively with resolution options:
  - Update the planning document to match reality
  - Create a follow-up task to address the gap
  - Mark as intentional deviation
  - Skip
- Produces a summary of all findings and actions taken

## 8. Check Progress

Check where things stand at any point.

```
Claude Code:  /belmont:status
Cursor:       Enable the belmont status rule, then: "Show belmont status"
Other:        Load skills/belmont/status.md as context
```

## 9. Iterate

After implementing a milestone:
- Run `/belmont:verify` to catch issues
- Run `/belmont:debug` for targeted fixes on specific issues found by verification (routes to auto or manual mode)
- Run `/belmont:next` to quickly fix follow-up tasks from verification
- Run `/belmont:review` to check alignment between plans and codebase
- Run `/belmont:implement` again for the next milestone
- Run `/belmont:status` to check progress
- Continue until all milestones are complete

## 10. Start Fresh

When you're done with a feature and want to plan something new:

```
Claude Code:  /belmont:reset
Cursor:       Enable the belmont reset rule, then: "Reset belmont state"
Other:        Load skills/belmont/reset.md as context
```

**What happens:**
- Agent reads current state and shows what will be cleared (feature name, tasks, milestones)
- Asks for explicit "yes" confirmation
- Resets PRD.md and PROGRESS.md to blank templates
- Deletes TECH_PLAN.md
- Prompts you to start fresh with `/belmont:product-plan`
