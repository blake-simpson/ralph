package main

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"
)

var (
	Version   = "dev"
	CommitSHA = "unknown"
	BuildDate = "unknown"
)

type taskStatus string

const (
	taskComplete   taskStatus = "complete"
	taskBlocked    taskStatus = "blocked"
	taskInProgress taskStatus = "in_progress"
	taskPending    taskStatus = "pending"
)

type task struct {
	ID     string
	Name   string
	Status taskStatus
}

type milestone struct {
	ID   string
	Name string
	Done bool
	Deps []string // e.g. ["M1", "M3"] — nil for no explicit deps
}

type featureSummary struct {
	Slug            string      `json:"slug"`
	Name            string      `json:"name"`
	TasksDone       int         `json:"tasks_done"`
	TasksTotal      int         `json:"tasks_total"`
	MilestonesDone  int         `json:"milestones_done"`
	MilestonesTotal int         `json:"milestones_total"`
	Milestones      []milestone `json:"milestones"`
	NextMilestone   *milestone  `json:"next_milestone,omitempty"`
	NextTask        *task       `json:"next_task,omitempty"`
	Blockers        []string    `json:"blockers,omitempty"`
	Status          string      `json:"status"`
	Deps            []string    `json:"deps,omitempty"`
	Priority        string      `json:"priority,omitempty"`
}

type statusReport struct {
	Feature         string
	TechPlanReady   bool
	PRFAQReady      bool
	OverallStatus   string
	TaskCounts      map[string]int
	Tasks           []task
	Milestones      []milestone
	Blockers        []string
	NextMilestone   *milestone
	NextTask        *task
	LastCompleted   *task
	RecentDecisions []string
	Features        []featureSummary
}

type cacheIndex struct {
	Root        string      `json:"root"`
	GeneratedAt time.Time   `json:"generated_at"`
	Files       []cacheFile `json:"files"`
}

type cacheFile struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

type findResult struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

type searchResult struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type config struct {
	Source string `json:"source"`
}

// Loop types
type loopActionType string

const (
	actionImplementMilestone loopActionType = "IMPLEMENT_MILESTONE"
	actionImplementNext      loopActionType = "IMPLEMENT_NEXT"
	actionVerify             loopActionType = "VERIFY"
	actionPause              loopActionType = "PAUSE"
	actionComplete           loopActionType = "COMPLETE"
	actionError              loopActionType = "ERROR"
	actionReplan             loopActionType = "REPLAN"
	actionSkipMilestone      loopActionType = "SKIP_MILESTONE"
	actionDebug              loopActionType = "DEBUG"
)

type loopAction struct {
	Type        loopActionType
	Reason      string
	MilestoneID string
}

type executionResult struct {
	Success    bool
	Output     string
	Error      string
	DurationMs int64
}

type workType string

const (
	workFrontend workType = "frontend" // .tsx, .jsx, .css, .scss, .html, .vue, .svelte
	workBackend  workType = "backend"  // .go, .py, .rs, .java, etc.
	workConfig   workType = "config"   // .yml, .yaml, .json, .toml, CI files
	workDocs     workType = "docs"     // .md, .txt
	workMixed    workType = "mixed"
	workMinimal  workType = "minimal"  // < 3 files changed
	workUnknown  workType = "unknown"
)

type historyEntry struct {
	Action       loopAction
	Result       *executionResult
	TasksDone    int
	TasksTotal   int
	MsDone       int
	MsTotal      int
	BlockerCount int
	HasFwlup     bool
	Iteration    int
	WorkType     workType
	FilesChanged int
	GitSHA       string
	PostGitSHA   string
}

type milestoneLoopState struct {
	ID           string
	Name         string
	Done         bool
	Implemented  bool
	Verified     bool
	VerifyFailed int
	WorkType     workType
	FilesChanged int
}

type checkpointPolicy string

const (
	policyAutonomous  checkpointPolicy = "autonomous"
	policyMilestone   checkpointPolicy = "milestone"
	policyEveryAction checkpointPolicy = "every_action"
)

type loopConfig struct {
	Feature       string
	Root          string
	Tool          string
	From          string
	To            string
	Policy        checkpointPolicy
	MaxIterations int
	MaxFailures   int
	MaxParallel   int
	DryRun        bool
}

type aiDecision struct {
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	MilestoneID string `json:"milestone_id,omitempty"`
}

type reconciliationFile struct {
	File            string `json:"file"`
	Confidence      string `json:"confidence"`
	Reason          string `json:"reason"`
	ConflictSummary string `json:"conflict_summary"`
	ResolvedContent string `json:"resolved_content"`
}

type reconciliationReport struct {
	Files []reconciliationFile `json:"files"`
}

func main() {
	// Clean up old binary on Windows after self-update
	if runtime.GOOS == "windows" {
		if exe, err := os.Executable(); err == nil {
			old := exe + ".old"
			if _, err := os.Stat(old); err == nil {
				os.Remove(old)
			}
		}
	}

	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "status":
		must(runStatus(os.Args[2:]))
	case "tree":
		must(runTree(os.Args[2:]))
	case "find":
		must(runFind(os.Args[2:]))
	case "search":
		must(runSearch(os.Args[2:]))
	case "auto", "loop":
		must(runAutoCmd(os.Args[2:]))
	case "install":
		must(runInstall(os.Args[2:]))
	case "update":
		must(runUpdate(os.Args[2:]))
	case "recover":
		must(runRecover(os.Args[2:]))
	case "version":
		fmt.Printf("belmont %s (%s, %s)\n", Version, CommitSHA, BuildDate)
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Belmont Helper")
	fmt.Fprintln(w, "==============")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  belmont install [--source PATH] [--project PATH] [--tools all|none|claude,codex,...]")
	fmt.Fprintln(w, "  belmont update [--check] [--force]")
	fmt.Fprintln(w, "  belmont status [--root PATH] [--feature SLUG] [--format text|json]")
	fmt.Fprintln(w, "  belmont tree [--root PATH] [--max-depth N] [--max-entries N] [--format text|json]")
	fmt.Fprintln(w, "  belmont find --name QUERY [--root PATH] [--regex] [--type file|dir|any] [--limit N] [--format text|json]")
	fmt.Fprintln(w, "  belmont search --pattern REGEX [--root PATH] [--limit N] [--format text|json]")
	fmt.Fprintln(w, "  belmont auto --feature SLUG [--from M1] [--to M5] [--tool claude|codex|gemini|copilot|cursor] [--policy autonomous|milestone|every_action] [--max-iterations N] [--max-parallel N] [--root PATH]")
	fmt.Fprintln(w, "    (alias: belmont loop)")
	fmt.Fprintln(w, "  belmont recover [--list] [--merge SLUG] [--clean SLUG] [--clean-all] [--root PATH] [--format text|json]")
	fmt.Fprintln(w, "  belmont version")
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runStatus(args []string) error {
	fsFlags := flag.NewFlagSet("status", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var root string
	var format string
	var maxName int
	var feature string
	fsFlags.StringVar(&root, "root", ".", "project root")
	fsFlags.StringVar(&format, "format", "text", "text or json")
	fsFlags.IntVar(&maxName, "max-task-name", 55, "max task name length")
	fsFlags.StringVar(&feature, "feature", "", "feature slug")
	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("status: %w", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	report, err := buildStatus(absRoot, maxName, feature)
	if err != nil {
		return err
	}

	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "text":
		fmt.Print(renderStatus(report))
		return nil
	default:
		return fmt.Errorf("status: unknown format %q", format)
	}
}

func buildStatus(root string, maxName int, feature string) (statusReport, error) {
	var report statusReport
	report.TaskCounts = map[string]int{
		"done":        0,
		"in_progress": 0,
		"blocked":     0,
		"pending":     0,
		"total":       0,
	}

	// Check for PR_FAQ
	prfaqPath := filepath.Join(root, ".belmont", "PR_FAQ.md")
	report.PRFAQReady = fileHasRealContent(prfaqPath)

	// Determine base path based on feature mode
	featuresDir := filepath.Join(root, ".belmont", "features")

	if feature != "" {
		// Specific feature requested
		featurePath := filepath.Join(featuresDir, feature)
		if !dirExists(featurePath) {
			return report, fmt.Errorf("status: feature %q not found in %s", feature, featuresDir)
		}

		prdPath := filepath.Join(featurePath, "PRD.md")
		progressPath := filepath.Join(featurePath, "PROGRESS.md")
		techPlanPath := filepath.Join(featurePath, "TECH_PLAN.md")

		prdContent, err := os.ReadFile(prdPath)
		if err != nil {
			return report, fmt.Errorf("status: missing %s", prdPath)
		}

		progressContent, err := os.ReadFile(progressPath)
		if err != nil {
			return report, fmt.Errorf("status: missing %s", progressPath)
		}

		report.Feature = extractFeatureName(string(prdContent))
		report.Tasks = parseTasks(string(prdContent), maxName)

		assignTaskStatuses(report.Tasks)
		report.TaskCounts["total"] = len(report.Tasks)
		for _, t := range report.Tasks {
			switch t.Status {
			case taskComplete:
				report.TaskCounts["done"]++
			case taskBlocked:
				report.TaskCounts["blocked"]++
			case taskInProgress:
				report.TaskCounts["in_progress"]++
			case taskPending:
				report.TaskCounts["pending"]++
			}
		}

		report.LastCompleted = lastCompletedTask(report.Tasks)
		report.Milestones = parseMilestones(string(progressContent))
		report.Blockers = parseBlockers(string(progressContent))
		report.RecentDecisions = parseDecisions(string(progressContent), 3)
		report.NextMilestone = nextMilestone(report.Milestones)
		report.NextTask = nextTask(report.Tasks)
		report.TechPlanReady = techPlanReady(techPlanPath)
		report.OverallStatus = computeOverallStatus(string(progressContent), report.Tasks)

		return report, nil
	}

	// Feature listing mode (default)
	features := listFeatures(featuresDir, maxName)
	if features == nil {
		features = []featureSummary{}
	}
	populateFeatureDeps(features, root)
	report.Features = features
	report.Feature = extractProductName(filepath.Join(root, ".belmont", "PRD.md"))
	report.TechPlanReady = techPlanReady(filepath.Join(root, ".belmont", "TECH_PLAN.md"))

	// Read master PROGRESS.md for overall status and blockers
	masterProgressPath := filepath.Join(root, ".belmont", "PROGRESS.md")
	if masterProgress, err := os.ReadFile(masterProgressPath); err == nil {
		content := string(masterProgress)
		statusLine := parseStatusLine(content)
		if statusLine != "" {
			report.OverallStatus = statusLine
		} else if len(features) > 0 {
			report.OverallStatus = computeFeatureListStatus(features)
		} else {
			report.OverallStatus = "🔴 Not Started"
		}
		report.Blockers = parseBlockers(content)
	} else if len(features) > 0 {
		report.OverallStatus = computeFeatureListStatus(features)
	} else {
		report.OverallStatus = "🔴 Not Started"
	}

	return report, nil
}

func extractProductName(prdPath string) string {
	content, err := os.ReadFile(prdPath)
	if err != nil {
		return "Unnamed Product"
	}
	re := regexp.MustCompile(`(?m)^#\s*Product:\s*(.+)$`)
	match := re.FindStringSubmatch(string(content))
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return extractFeatureName(string(content))
}

func listFeatures(featuresDir string, maxName int) []featureSummary {
	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return nil
	}
	var features []featureSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		featurePath := filepath.Join(featuresDir, slug)
		prdPath := filepath.Join(featurePath, "PRD.md")

		name := slug
		prdContent, err := os.ReadFile(prdPath)
		if err == nil {
			extracted := extractFeatureName(string(prdContent))
			if extracted != "Unknown" {
				name = extracted
			}
		}

		tasksDone := 0
		tasksTotal := 0
		if err == nil {
			tasks := parseTasks(string(prdContent), maxName)
			assignTaskStatuses(tasks)
			tasksTotal = len(tasks)
			for _, t := range tasks {
				if t.Status == taskComplete {
					tasksDone++
				}
			}
		}

		var milestones []milestone
		var featureBlockers []string
		milestonesDone := 0
		milestonesTotal := 0
		progressPath := filepath.Join(featurePath, "PROGRESS.md")
		if progressContent, err := os.ReadFile(progressPath); err == nil {
			milestones = parseMilestones(string(progressContent))
			milestonesTotal = len(milestones)
			for _, m := range milestones {
				if m.Done {
					milestonesDone++
				}
			}
			featureBlockers = parseBlockers(string(progressContent))
		}

		// Compute next milestone and next task for this feature
		var featureNextMilestone *milestone
		var featureNextTask *task
		if err == nil {
			tasks := parseTasks(string(prdContent), maxName)
			assignTaskStatuses(tasks)
			featureNextTask = nextTask(tasks)
		}
		featureNextMilestone = nextMilestone(milestones)

		status := "🔴 Not Started"
		allTasksDone := tasksTotal > 0 && tasksDone == tasksTotal
		allMilestonesDone := milestonesTotal > 0 && milestonesDone == milestonesTotal
		if allTasksDone && (milestonesTotal == 0 || allMilestonesDone) {
			status = "✅ Complete"
		} else if tasksDone > 0 || milestonesDone > 0 {
			status = "🟡 In Progress"
		}

		features = append(features, featureSummary{
			Slug:            slug,
			Name:            name,
			TasksDone:       tasksDone,
			TasksTotal:      tasksTotal,
			MilestonesDone:  milestonesDone,
			MilestonesTotal: milestonesTotal,
			Milestones:      milestones,
			NextMilestone:   featureNextMilestone,
			NextTask:        featureNextTask,
			Blockers:        featureBlockers,
			Status:          status,
		})
	}
	return features
}

// parseMasterPRDDeps reads the master PRD and extracts feature slug → dependency slugs mapping
// from the ## Features table. Handles "None", empty, and comma-separated slugs.
func parseMasterPRDDeps(root string) (deps map[string][]string, priorities map[string]string) {
	deps = make(map[string][]string)
	priorities = make(map[string]string)

	prdPath := filepath.Join(root, ".belmont", "PRD.md")
	content, err := os.ReadFile(prdPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of Features table
		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}

		// Stop at next heading
		if inTable && strings.HasPrefix(trimmed, "## ") {
			break
		}

		if !inTable || !strings.HasPrefix(trimmed, "|") {
			continue
		}

		// Skip header and separator rows
		cols := strings.Split(trimmed, "|")
		// Remove empty first/last from leading/trailing pipes
		var cells []string
		for _, c := range cols {
			c = strings.TrimSpace(c)
			if c != "" {
				cells = append(cells, c)
			}
		}

		// Need at least 4 columns: Feature, Slug, Priority, Dependencies
		if len(cells) < 4 {
			continue
		}

		// Skip header row and separator
		slug := strings.TrimSpace(cells[1])
		if slug == "Slug" || strings.HasPrefix(slug, "-") || strings.HasPrefix(slug, ":") {
			continue
		}

		priority := strings.TrimSpace(cells[2])
		depStr := strings.TrimSpace(cells[3])

		priorities[slug] = priority

		if depStr == "" || strings.EqualFold(depStr, "None") || depStr == "-" {
			continue
		}

		// Parse comma-separated dependency slugs
		var depSlugs []string
		for _, d := range strings.Split(depStr, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				depSlugs = append(depSlugs, d)
			}
		}
		if len(depSlugs) > 0 {
			deps[slug] = depSlugs
		}
	}
	return
}

// populateFeatureDeps enriches feature summaries with dependency and priority info from master PRD.
func populateFeatureDeps(features []featureSummary, root string) {
	deps, priorities := parseMasterPRDDeps(root)
	for i := range features {
		if d, ok := deps[features[i].Slug]; ok {
			features[i].Deps = d
		}
		if p, ok := priorities[features[i].Slug]; ok {
			features[i].Priority = p
		}
	}
}

func computeFeatureListStatus(features []featureSummary) string {
	allComplete := true
	anyProgress := false
	for _, f := range features {
		if f.Status != "✅ Complete" {
			allComplete = false
		}
		if f.TasksDone > 0 {
			anyProgress = true
		}
	}
	if allComplete && len(features) > 0 {
		return "✅ Complete"
	}
	if anyProgress {
		return "🟡 In Progress"
	}
	return "🔴 Not Started"
}

func extractFeatureName(prd string) string {
	re := regexp.MustCompile(`(?m)^#\s*PRD:\s*(.+)$`)
	match := re.FindStringSubmatch(prd)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return "Unknown"
}

func parseTasks(prd string, maxName int) []task {
	re := regexp.MustCompile(`(?m)^###\s+(P\d+-[\w][\w-]*):\s*(.+)$`)
	matches := re.FindAllStringSubmatch(prd, -1)
	tasks := make([]task, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		id := strings.TrimSpace(match[1])
		raw := strings.TrimSpace(match[2])
		status := detectTaskStatus(raw)
		cleaned := normalizeTaskName(raw)
		if maxName > 0 && len([]rune(cleaned)) > maxName {
			cleaned = string([]rune(cleaned)[:maxName-1]) + "…"
		}
		tasks = append(tasks, task{ID: id, Name: cleaned, Status: status})
	}

	sort.Slice(tasks, func(i, j int) bool {
		pi, ni := parseTaskOrder(tasks[i].ID)
		pj, nj := parseTaskOrder(tasks[j].ID)
		if pi != pj {
			return pi < pj
		}
		return ni < nj
	})

	return tasks
}

func normalizeTaskName(name string) string {
	cleaned := name
	cleaned = strings.ReplaceAll(cleaned, "✅", "")
	cleaned = strings.ReplaceAll(cleaned, "🚫", "")
	cleaned = strings.ReplaceAll(cleaned, "🔄", "")
	cleaned = strings.ReplaceAll(cleaned, "⬜", "")

	cleaned = regexp.MustCompile(`(?i)\[done\]`).ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`(?i)blocked`).ReplaceAllString(cleaned, "")
	cleaned = regexp.MustCompile(`(?i)follow-?up`).ReplaceAllString(cleaned, "")

	return strings.TrimSpace(cleaned)
}

func assignTaskStatuses(tasks []task) {
	inProgressAssigned := false
	for i := range tasks {
		status := tasks[i].Status
		if status == taskPending && !inProgressAssigned {
			status = taskInProgress
			inProgressAssigned = true
		}
		tasks[i].Status = status
	}
}

func detectTaskStatus(name string) taskStatus {
	if strings.Contains(name, "✅") || regexp.MustCompile(`(?i)\[done\]`).MatchString(name) {
		return taskComplete
	}
	if strings.Contains(name, "🚫") || regexp.MustCompile(`(?i)blocked`).MatchString(name) {
		return taskBlocked
	}
	return taskPending
}

func parseTaskOrder(id string) (int, int) {
	re := regexp.MustCompile(`^P(\d+)-(\d+)`)
	match := re.FindStringSubmatch(id)
	if len(match) != 3 {
		return 99, 99
	}
	return atoiDefault(match[1], 99), atoiDefault(match[2], 99)
}

func lastCompletedTask(tasks []task) *task {
	var last *task
	for i := range tasks {
		if tasks[i].Status == taskComplete {
			t := tasks[i]
			last = &t
		}
	}
	return last
}

func parseMilestones(progress string) []milestone {
	re := regexp.MustCompile(`(?m)^###\s+([✅⬜🔄🚫])?\s*M(\d+):\s*(.+)$`)
	depsRe := regexp.MustCompile(`\(depends:\s*(M[\d]+(?:\s*,\s*M[\d]+)*)\)\s*$`)
	matches := re.FindAllStringSubmatch(progress, -1)
	milestones := make([]milestone, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		marker := strings.TrimSpace(match[1])
		id := "M" + strings.TrimSpace(match[2])
		name := strings.TrimSpace(match[3])
		done := marker == "✅"

		// Extract dependency annotations from name
		var deps []string
		if depsMatch := depsRe.FindStringSubmatch(name); len(depsMatch) >= 2 {
			name = strings.TrimSpace(depsRe.ReplaceAllString(name, ""))
			for _, d := range strings.Split(depsMatch[1], ",") {
				deps = append(deps, strings.TrimSpace(d))
			}
		}

		milestones = append(milestones, milestone{ID: id, Name: name, Done: done, Deps: deps})
	}
	return milestones
}

func nextMilestone(milestones []milestone) *milestone {
	for _, m := range milestones {
		if !m.Done {
			mm := m
			return &mm
		}
	}
	return nil
}

func nextTask(tasks []task) *task {
	for _, t := range tasks {
		if t.Status == taskInProgress || t.Status == taskPending {
			tt := t
			return &tt
		}
	}
	return nil
}

func parseBlockers(progress string) []string {
	return parseSectionLines(progress, "## Blockers")
}

func parseDecisions(progress string, limit int) []string {
	lines := parseSectionLines(progress, "## Decisions Log")
	if len(lines) <= limit {
		return lines
	}
	return lines[len(lines)-limit:]
}

func parseSectionLines(doc, header string) []string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(header) + `\s*$`)
	loc := re.FindStringIndex(doc)
	if loc == nil {
		return nil
	}
	rest := doc[loc[1]:]
	lines := strings.Split(rest, "\n")
	var results []string
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		if trimmed == "" {
			continue
		}
		if strings.Contains(strings.ToLower(trimmed), "none") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "-")
		trimmed = strings.TrimPrefix(trimmed, "*")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			results = append(results, trimmed)
		}
	}
	return results
}

func techPlanReady(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(content)) != ""
}

// fileHasRealContent checks if a file exists and has content beyond template/placeholder text.
func fileHasRealContent(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return false
	}
	// Check for known template/placeholder texts
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "run /belmont:") || strings.HasPrefix(lower, "run the /belmont:") {
		return false
	}
	return true
}

func computeOverallStatus(progress string, tasks []task) string {
	statusLine := parseStatusLine(progress)
	if strings.Contains(strings.ToLower(statusLine), "blocked") {
		if strings.TrimSpace(statusLine) != "" {
			return statusLine
		}
		return "🔴 BLOCKED"
	}

	if len(tasks) == 0 {
		return "🔴 Not Started"
	}

	allDoneOrBlocked := true
	anyDone := false
	anyInProgress := false

	for _, t := range tasks {
		if t.Status == taskComplete {
			anyDone = true
		}
		if t.Status == taskInProgress {
			anyInProgress = true
		}
		if t.Status != taskComplete && t.Status != taskBlocked {
			allDoneOrBlocked = false
		}
	}

	if allDoneOrBlocked {
		return "✅ Complete"
	}
	if anyDone || anyInProgress {
		return "🟡 In Progress"
	}
	return "🔴 Not Started"
}

func parseStatusLine(progress string) string {
	re := regexp.MustCompile(`(?m)^##\s*Status:\s*(.+)$`)
	match := re.FindStringSubmatch(progress)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func renderStatus(report statusReport) string {
	// Feature listing mode (default when no --feature specified)
	if report.Features != nil {
		return renderFeatureListing(report)
	}

	techPlan := "⚠ Not written (run /belmont:tech-plan to create)"
	if report.TechPlanReady {
		techPlan = "✅ Ready"
	}

	taskLine := fmt.Sprintf("Tasks: %d done, %d in progress, %d blocked, %d pending (of %d total)",
		report.TaskCounts["done"],
		report.TaskCounts["in_progress"],
		report.TaskCounts["blocked"],
		report.TaskCounts["pending"],
		report.TaskCounts["total"],
	)

	var sb strings.Builder
	sb.WriteString("Belmont Status\n")
	sb.WriteString("==============\n\n")
	sb.WriteString(fmt.Sprintf("Feature: %s\n\n", report.Feature))
	sb.WriteString(fmt.Sprintf("Tech Plan: %s\n\n", techPlan))
	sb.WriteString(fmt.Sprintf("Status: %s\n\n", report.OverallStatus))
	sb.WriteString(taskLine)
	sb.WriteString("\n\n")

	if len(report.Tasks) > 0 {
		for _, t := range report.Tasks {
			sb.WriteString(fmt.Sprintf("  %s %s: %s\n", taskStatusIcon(t.Status), t.ID, t.Name))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Milestones:\n")
	if len(report.Milestones) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, m := range report.Milestones {
			icon := "⬜"
			if m.Done {
				icon = "✅"
			}
			sb.WriteString(fmt.Sprintf("  %s %s: %s\n", icon, m.ID, m.Name))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Active Blockers:\n")
	if len(report.Blockers) == 0 {
		sb.WriteString("  - None\n")
	} else {
		for _, b := range report.Blockers {
			sb.WriteString(fmt.Sprintf("  - %s\n", b))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Next Milestone:\n")
	if report.NextMilestone == nil {
		sb.WriteString("  - None\n")
	} else {
		sb.WriteString(fmt.Sprintf("  - %s - %s\n", report.NextMilestone.ID, report.NextMilestone.Name))
	}
	sb.WriteString("Next Individual Task:\n")
	if report.NextTask == nil {
		sb.WriteString("  - None\n")
	} else {
		sb.WriteString(fmt.Sprintf("  - %s - %s\n", report.NextTask.ID, report.NextTask.Name))
	}
	sb.WriteString("\n")

	sb.WriteString("Recent Activity:\n")
	sb.WriteString("---\n")
	if report.LastCompleted == nil {
		sb.WriteString("Last completed: None\n")
	} else {
		sb.WriteString(fmt.Sprintf("Last completed: %s - %s\n", report.LastCompleted.ID, report.LastCompleted.Name))
	}
	sb.WriteString("Recent decisions:\n")
	if len(report.RecentDecisions) == 0 {
		sb.WriteString("  - None\n")
	} else {
		for _, d := range report.RecentDecisions {
			sb.WriteString(fmt.Sprintf("  - %s\n", d))
		}
	}
	return sb.String()
}

func renderFeatureListing(report statusReport) string {
	prfaq := "⚠ Not written (run /belmont:working-backwards)"
	if report.PRFAQReady {
		prfaq = "✅ Written"
	}
	techPlan := "⚠ Not written"
	if report.TechPlanReady {
		techPlan = "✅ Ready"
	}

	var sb strings.Builder
	sb.WriteString("Belmont Status\n")
	sb.WriteString("==============\n\n")
	sb.WriteString(fmt.Sprintf("Product: %s\n\n", report.Feature))
	sb.WriteString(fmt.Sprintf("PR/FAQ: %s\n", prfaq))
	sb.WriteString(fmt.Sprintf("Master Tech Plan: %s\n\n", techPlan))
	sb.WriteString(fmt.Sprintf("Status: %s\n\n", report.OverallStatus))

	if len(report.Features) == 0 {
		sb.WriteString("Features:\n")
		sb.WriteString("  (none — run /belmont:product-plan to create your first feature)\n")
	} else {
		for _, f := range report.Features {
			icon := "🔴"
			if f.Status == "✅ Complete" {
				icon = "✅"
			} else if f.Status == "🟡 In Progress" {
				icon = "🟡"
			}
			sb.WriteString(fmt.Sprintf("%s %s (%s)\n", icon, f.Name, f.Slug))
			sb.WriteString(fmt.Sprintf("  Tasks: %d/%d done", f.TasksDone, f.TasksTotal))
			if f.MilestonesTotal > 0 {
				sb.WriteString(fmt.Sprintf("  |  Milestones: %d/%d done", f.MilestonesDone, f.MilestonesTotal))
			}
			sb.WriteString("\n")

			// Show milestone listing
			if len(f.Milestones) > 0 {
				for _, m := range f.Milestones {
					mIcon := "⬜"
					if m.Done {
						mIcon = "✅"
					} else if f.NextMilestone != nil && m.ID == f.NextMilestone.ID {
						mIcon = "🔄"
					}
					sb.WriteString(fmt.Sprintf("    %s %s: %s\n", mIcon, m.ID, m.Name))
				}
			}

			// Show next task if feature is in progress
			if f.NextTask != nil && f.Status == "🟡 In Progress" {
				sb.WriteString(fmt.Sprintf("  Next: %s — %s\n", f.NextTask.ID, f.NextTask.Name))
			}

			// Show blockers if any
			if len(f.Blockers) > 0 {
				sb.WriteString("  Blockers:\n")
				for _, b := range f.Blockers {
					sb.WriteString(fmt.Sprintf("    - %s\n", b))
				}
			}

			sb.WriteString("\n")
		}
	}

	// Show master-level blockers if any (from master PROGRESS.md)
	if len(report.Blockers) > 0 {
		sb.WriteString("Active Blockers:\n")
		for _, b := range report.Blockers {
			sb.WriteString(fmt.Sprintf("  - %s\n", b))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Use --feature <slug> for detailed task-level status.\n")
	return sb.String()
}

func taskStatusIcon(status taskStatus) string {
	switch status {
	case taskComplete:
		return "✅"
	case taskBlocked:
		return "🚫"
	case taskInProgress:
		return "🔄"
	default:
		return "⬜"
	}
}

func runTree(args []string) error {
	fsFlags := flag.NewFlagSet("tree", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var root string
	var maxDepth int
	var maxEntries int
	var format string
	var cacheTTL time.Duration
	fsFlags.StringVar(&root, "root", ".", "project root")
	fsFlags.IntVar(&maxDepth, "max-depth", 2, "max depth")
	fsFlags.IntVar(&maxEntries, "max-entries", 200, "max entries")
	fsFlags.StringVar(&format, "format", "text", "text or json")
	fsFlags.DurationVar(&cacheTTL, "cache-ttl", 5*time.Second, "cache ttl")
	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("tree: %w", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	entries, err := listEntries(absRoot, maxDepth, maxEntries, cacheTTL)
	if err != nil {
		return err
	}

	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	case "text":
		for _, entry := range entries {
			indent := strings.Repeat("  ", entry.Depth)
			name := filepath.Base(entry.Path)
			if entry.IsDir {
				fmt.Printf("%s%s/\n", indent, name)
			} else {
				fmt.Printf("%s%s\n", indent, name)
			}
		}
		return nil
	default:
		return fmt.Errorf("tree: unknown format %q", format)
	}
}

func runFind(args []string) error {
	fsFlags := flag.NewFlagSet("find", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var root string
	var name string
	var useRegex bool
	var matchType string
	var limit int
	var format string
	var ignoreCase bool
	var cacheTTL time.Duration

	fsFlags.StringVar(&root, "root", ".", "project root")
	fsFlags.StringVar(&name, "name", "", "name query")
	fsFlags.BoolVar(&useRegex, "regex", false, "treat name as regex")
	fsFlags.StringVar(&matchType, "type", "file", "file, dir, or any")
	fsFlags.IntVar(&limit, "limit", 200, "max results")
	fsFlags.StringVar(&format, "format", "text", "text or json")
	fsFlags.BoolVar(&ignoreCase, "ignore-case", false, "case insensitive")
	fsFlags.DurationVar(&cacheTTL, "cache-ttl", 5*time.Second, "cache ttl")

	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("find: %w", err)
	}

	if name == "" {
		return errors.New("find: --name is required")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	matcher, err := buildMatcher(name, useRegex, ignoreCase)
	if err != nil {
		return err
	}

	entries, err := listEntries(absRoot, -1, limit, cacheTTL)
	if err != nil {
		return err
	}

	matchType = strings.ToLower(matchType)
	results := make([]findResult, 0)
	for _, entry := range entries {
		if limit > 0 && len(results) >= limit {
			break
		}
		if matchType == "file" && entry.IsDir {
			continue
		}
		if matchType == "dir" && !entry.IsDir {
			continue
		}
		rel := entry.Path
		namePart := filepath.Base(rel)
		if matcher(namePart) {
			results = append(results, findResult{Path: rel, IsDir: entry.IsDir})
		}
	}

	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	case "text":
		for _, res := range results {
			fmt.Println(res.Path)
		}
		return nil
	default:
		return fmt.Errorf("find: unknown format %q", format)
	}
}

func runSearch(args []string) error {
	fsFlags := flag.NewFlagSet("search", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var root string
	var pattern string
	var limit int
	var format string
	var ignoreCase bool
	fsFlags.StringVar(&root, "root", ".", "project root")
	fsFlags.StringVar(&pattern, "pattern", "", "regex pattern")
	fsFlags.IntVar(&limit, "limit", 200, "max results")
	fsFlags.StringVar(&format, "format", "text", "text or json")
	fsFlags.BoolVar(&ignoreCase, "ignore-case", false, "case insensitive")

	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if pattern == "" {
		return errors.New("search: --pattern is required")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("search: invalid pattern: %w", err)
	}

	results := make([]searchResult, 0)
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == absRoot {
			return nil
		}
		if shouldSkip(path, d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if limit > 0 && len(results) >= limit {
			return io.EOF
		}

		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}

		matches, err := searchFile(path, rel, re, limit-len(results))
		if err != nil {
			return err
		}
		results = append(results, matches...)
		return nil
	})

	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	case "text":
		for _, res := range results {
			fmt.Printf("%s:%d:%s\n", res.Path, res.Line, res.Text)
		}
		return nil
	default:
		return fmt.Errorf("search: unknown format %q", format)
	}
}

type toolConfig struct {
	Name  string
	Label string
}

var toolConfigs = []toolConfig{
	{Name: "claude", Label: "Claude Code (.claude/)"},
	{Name: "codex", Label: "Codex (.codex/)"},
	{Name: "cursor", Label: "Cursor (.cursor/)"},
	{Name: "windsurf", Label: "Windsurf (.windsurf/)"},
	{Name: "gemini", Label: "Gemini (.gemini/)"},
	{Name: "copilot", Label: "GitHub Copilot (.copilot/)"},
}

func runInstall(args []string) error {
	fsFlags := flag.NewFlagSet("install", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var source string
	var project string
	var toolsFlag string
	var noPrompt bool
	fsFlags.StringVar(&source, "source", "", "belmont source directory")
	fsFlags.StringVar(&project, "project", ".", "project directory")
	fsFlags.StringVar(&toolsFlag, "tools", "", "all|none|comma list")
	fsFlags.BoolVar(&noPrompt, "no-prompt", false, "disable interactive prompts")
	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	projectRoot, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	// Determine mode: embedded (release binary) vs source (developer)
	useEmbedded := (source == "" && os.Getenv("BELMONT_SOURCE") == "") && hasEmbeddedFiles

	fmt.Println("Belmont Project Setup")
	fmt.Println("=====================")
	fmt.Println("")
	fmt.Printf("Project: %s\n", projectRoot)
	fmt.Println("")

	selectedTools, err := resolveTools(projectRoot, toolsFlag, noPrompt)
	if err != nil {
		return err
	}

	if useEmbedded {
		fmt.Println("Installing agents to .agents/belmont/...")
		if err := syncEmbeddedDir(embeddedAgents, "agents/belmont", filepath.Join(projectRoot, ".agents", "belmont")); err != nil {
			return err
		}
		fmt.Println("")

		fmt.Println("Installing skills to .agents/skills/belmont/...")
		if err := syncEmbeddedDir(embeddedSkills, "skills/belmont", filepath.Join(projectRoot, ".agents", "skills", "belmont")); err != nil {
			return err
		}
		fmt.Println("")
	} else {
		sourceRoot, err := resolveSourceRoot(source)
		if err != nil {
			return err
		}

		agentsSource := filepath.Join(sourceRoot, "agents", "belmont")
		skillsSource := filepath.Join(sourceRoot, "skills", "belmont")

		if !dirExists(agentsSource) || !dirExists(skillsSource) {
			return fmt.Errorf("install: source missing agents/ or skills/ in %s", sourceRoot)
		}

		fmt.Println("Installing agents to .agents/belmont/...")
		if err := syncMarkdownDir(agentsSource, filepath.Join(projectRoot, ".agents", "belmont")); err != nil {
			return err
		}
		fmt.Println("")

		fmt.Println("Installing skills to .agents/skills/belmont/...")
		if err := syncMarkdownDir(skillsSource, filepath.Join(projectRoot, ".agents", "skills", "belmont")); err != nil {
			return err
		}
		fmt.Println("")
	}

	if containsTool(selectedTools, "codex") {
		fmt.Println("Updating AGENTS.md for Codex skill routing...")
		if changed, err := ensureCodexAgentsGuidance(projectRoot); err != nil {
			return err
		} else if changed {
			fmt.Println("  + AGENTS.md Belmont Codex skill routing section")
		} else {
			fmt.Println("  = AGENTS.md Belmont Codex skill routing section (unchanged)")
		}
		if removed, err := removeLegacyCodexSkillsIndex(projectRoot); err != nil {
			return err
		} else if removed {
			fmt.Println("  - SKILLS.md (removed legacy Belmont Codex index)")
		}
		fmt.Println("")
	}

	for _, tool := range selectedTools {
		if err := setupTool(projectRoot, tool); err != nil {
			return err
		}
		fmt.Println("")
	}

	if len(selectedTools) == 0 {
		fmt.Println("Skipped tool linking.")
		fmt.Println("Skills are in .agents/skills/belmont/ -- reference them from your tool.")
		fmt.Println("")
	}

	if err := ensureStateFiles(projectRoot); err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("Belmont installed!")
	fmt.Println("")
	fmt.Println("Agents:  .agents/belmont/")
	fmt.Println("Skills:  .agents/skills/belmont/")
	fmt.Println("State:   .belmont/")
	if fileExists(filepath.Join(projectRoot, ".belmont", "bin", "belmont")) || fileExists(filepath.Join(projectRoot, ".belmont", "bin", "belmont.exe")) {
		fmt.Println("Helper:  .belmont/bin/belmont")
	}

	if len(selectedTools) > 0 {
		fmt.Println("")
		fmt.Println("Tool integrations:")
		for _, tool := range selectedTools {
			switch tool {
			case "claude":
				fmt.Println("  Claude Code  .claude/agents/belmont -> ../../.agents/belmont")
				fmt.Println("              .claude/commands/belmont (copied from .agents/skills/belmont)")
				fmt.Println("    Use: /belmont:working-backwards, /belmont:product-plan, /belmont:tech-plan, /belmont:implement, /belmont:next, /belmont:verify, /belmont:debug, /belmont:debug-auto, /belmont:debug-manual, /belmont:status")
			case "codex":
				fmt.Println("  Codex        .codex/belmont (copied from .agents/skills/belmont)")
				fmt.Println("    Use: AGENTS.md includes Belmont skill routing for belmont:<skill> prompts")
			case "cursor":
				fmt.Println("  Cursor       .cursor/rules/belmont/*.mdc -> .agents/skills/belmont/*.md")
				fmt.Println("    Use: Reference belmont rules in Composer/Agent, or toggle in Settings > Rules")
			case "windsurf":
				fmt.Println("  Windsurf     .windsurf/rules/belmont -> .agents/skills/belmont")
				fmt.Println("    Use: Reference belmont rules in Cascade")
			case "gemini":
				fmt.Println("  Gemini       .gemini/rules/belmont -> .agents/skills/belmont")
				fmt.Println("    Use: Reference belmont rules in Gemini")
			case "copilot":
				fmt.Println("  Copilot      .copilot/belmont -> .agents/skills/belmont")
				fmt.Println("    Use: Reference belmont files in Copilot Chat")
			}
		}
	}

	fmt.Println("")
	fmt.Println("Workflow:")
	fmt.Println("  0. PR/FAQ     - Define product vision (Working Backwards)")
	fmt.Println("  1. Plan       - Create PRD interactively")
	fmt.Println("  2. Tech Plan  - Create technical implementation plan")
	fmt.Println("  3. Implement  - Implement next milestone (full pipeline)")
	fmt.Println("  4. Next       - Implement next single task (lightweight)")
	fmt.Println("  5. Verify     - Run verification and code review")
	fmt.Println("  6. Status     - View progress")
	fmt.Println("  7. Reset      - Reset state and start fresh")
	fmt.Println("")

	return nil
}

func searchFile(path, rel string, re *regexp.Regexp, limit int) ([]searchResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if isBinary(file) {
		return nil, nil
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
	var results []searchResult
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if limit > 0 && len(results) >= limit {
			break
		}
		line := scanner.Text()
		if re.MatchString(line) {
			results = append(results, searchResult{Path: rel, Line: lineNo, Text: line})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func isBinary(file *os.File) bool {
	buf := make([]byte, 8000)
	n, _ := file.Read(buf)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return true
	}
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}

type listEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Depth int    `json:"depth"`
}

func listEntries(root string, maxDepth int, maxEntries int, cacheTTL time.Duration) ([]listEntry, error) {
	useCache := cacheTTL > 0
	cachePath := filepath.Join(root, ".belmont", "cache", "index.json")

	if useCache {
		if entries, ok := loadCachedEntries(cachePath, root, cacheTTL); ok {
			return filterEntries(entries, maxDepth, maxEntries), nil
		}
	}

	entries := make([]listEntry, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if shouldSkip(path, d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		depth := depthOf(rel)
		if maxDepth >= 0 && depth > maxDepth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		entries = append(entries, listEntry{Path: rel, IsDir: d.IsDir(), Depth: depth})
		if maxEntries > 0 && len(entries) >= maxEntries {
			return io.EOF
		}
		return nil
	})

	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	if useCache {
		_ = saveCache(cachePath, root, entries)
	}

	return entries, nil
}

func saveCache(path, root string, entries []listEntry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	cache := cacheIndex{Root: root, GeneratedAt: time.Now(), Files: make([]cacheFile, 0, len(entries))}
	for _, entry := range entries {
		cache.Files = append(cache.Files, cacheFile{Path: entry.Path, IsDir: entry.IsDir})
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(cache)
}

func loadCachedEntries(path, root string, ttl time.Duration) ([]listEntry, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()

	var cache cacheIndex
	dec := json.NewDecoder(file)
	if err := dec.Decode(&cache); err != nil {
		return nil, false
	}

	if cache.Root != root {
		return nil, false
	}
	if time.Since(cache.GeneratedAt) > ttl {
		return nil, false
	}

	entries := make([]listEntry, 0, len(cache.Files))
	for _, file := range cache.Files {
		entries = append(entries, listEntry{Path: file.Path, IsDir: file.IsDir, Depth: depthOf(file.Path)})
	}
	return entries, true
}

func filterEntries(entries []listEntry, maxDepth int, maxEntries int) []listEntry {
	filtered := make([]listEntry, 0, len(entries))
	for _, entry := range entries {
		if maxDepth >= 0 && entry.Depth > maxDepth {
			continue
		}
		filtered = append(filtered, entry)
		if maxEntries > 0 && len(filtered) >= maxEntries {
			break
		}
	}
	return filtered
}

func depthOf(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	sep := string(os.PathSeparator)
	return strings.Count(rel, sep)
}

func buildMatcher(pattern string, regex bool, ignoreCase bool) (func(string) bool, error) {
	if regex {
		if ignoreCase {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	}

	if ignoreCase {
		lower := strings.ToLower(pattern)
		return func(s string) bool { return strings.Contains(strings.ToLower(s), lower) }, nil
	}
	return func(s string) bool { return strings.Contains(s, pattern) }, nil
}

func shouldSkip(path string, d fs.DirEntry) bool {
	name := d.Name()
	if d.IsDir() {
		switch name {
		case ".git", ".belmont", "node_modules", "dist", "build", "out", ".next", ".turbo", ".cache", "vendor", "coverage", ".idea", ".vscode", "target", "tmp", "temp", "__pycache__":
			return true
		}
		return false
	}

	switch name {
	case ".DS_Store", "Thumbs.db":
		return true
	}
	return false
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func resolveSourceRoot(source string) (string, error) {
	if source != "" {
		abs, err := filepath.Abs(source)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	if env := strings.TrimSpace(os.Getenv("BELMONT_SOURCE")); env != "" {
		abs, err := filepath.Abs(env)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	if cfgSource, ok := loadConfigSource(); ok {
		abs, err := filepath.Abs(cfgSource)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exePath)
	for i := 0; i < 6; i++ {
		skills := filepath.Join(dir, "skills", "belmont")
		agents := filepath.Join(dir, "agents", "belmont")
		if dirExists(skills) && dirExists(agents) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New("install: unable to locate belmont source; pass --source PATH")
}

func loadConfigSource() (string, bool) {
	paths := configPaths()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg config
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if strings.TrimSpace(cfg.Source) != "" {
			return cfg.Source, true
		}
	}
	return "", false
}

func configPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	paths := []string{
		filepath.Join(home, ".config", "belmont", "config.json"),
		filepath.Join(home, ".belmont", "config.json"),
	}
	return paths
}

func resolveTools(projectRoot, toolsFlag string, noPrompt bool) ([]string, error) {
	detected := detectTools(projectRoot)

	if toolsFlag != "" {
		switch strings.ToLower(toolsFlag) {
		case "all":
			if len(detected) > 0 {
				return detected, nil
			}
			return allToolNames(), nil
		case "none":
			return nil, nil
		default:
			parts := strings.Split(toolsFlag, ",")
			var selected []string
			for _, p := range parts {
				name := strings.TrimSpace(strings.ToLower(p))
				if name == "" {
					continue
				}
				if !isKnownTool(name) {
					return nil, fmt.Errorf("install: unknown tool %q", name)
				}
				selected = append(selected, name)
			}
			return selected, nil
		}
	}

	if noPrompt {
		if len(detected) > 0 {
			return detected, nil
		}
		return nil, nil
	}

	reader := bufio.NewReader(os.Stdin)

	if len(detected) > 0 {
		fmt.Println("Detected AI tools:")
		for i, tool := range detected {
			fmt.Printf("  [%d] %s\n", i+1, toolLabel(tool))
		}
		fmt.Println("")
		fmt.Println("Install skills for:")
		fmt.Println("  [a] All detected tools")
		for i, tool := range detected {
			fmt.Printf("  [%d] %s only\n", i+1, toolLabel(tool))
		}
		fmt.Println("  [s] Skip (install agents only)")
		fmt.Println("")
		fmt.Print("Choice [a]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		fmt.Println("")

		if choice == "" || strings.EqualFold(choice, "a") {
			return detected, nil
		}
		if strings.EqualFold(choice, "s") {
			return nil, nil
		}
		if idx, err := strconv.Atoi(choice); err == nil {
			if idx >= 1 && idx <= len(detected) {
				return []string{detected[idx-1]}, nil
			}
		}
		return detected, nil
	}

	fmt.Println("No AI tool directories detected.")
	fmt.Println("")
	fmt.Println("Which tool are you using?")
	for i, tool := range toolConfigs {
		fmt.Printf("  [%d] %s\n", i+1, tool.Label)
	}
	fmt.Println("  [s] Skip (install agents only - reference files manually)")
	fmt.Println("")
	fmt.Print("Choice: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	fmt.Println("")

	if strings.EqualFold(choice, "s") {
		return nil, nil
	}
	if idx, err := strconv.Atoi(choice); err == nil {
		if idx >= 1 && idx <= len(toolConfigs) {
			return []string{toolConfigs[idx-1].Name}, nil
		}
	}

	fmt.Println("Invalid choice. Installing agents only.")
	return nil, nil
}

func detectTools(projectRoot string) []string {
	var detected []string
	if dirExists(filepath.Join(projectRoot, ".claude")) {
		detected = append(detected, "claude")
	}
	if dirExists(filepath.Join(projectRoot, ".codex")) {
		detected = append(detected, "codex")
	}
	if dirExists(filepath.Join(projectRoot, ".cursor")) {
		detected = append(detected, "cursor")
	}
	if dirExists(filepath.Join(projectRoot, ".windsurf")) {
		detected = append(detected, "windsurf")
	}
	if dirExists(filepath.Join(projectRoot, ".gemini")) {
		detected = append(detected, "gemini")
	}
	if dirExists(filepath.Join(projectRoot, ".copilot")) {
		detected = append(detected, "copilot")
	}
	return detected
}

func allToolNames() []string {
	names := make([]string, 0, len(toolConfigs))
	for _, tool := range toolConfigs {
		names = append(names, tool.Name)
	}
	return names
}

func isKnownTool(name string) bool {
	for _, tool := range toolConfigs {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func toolLabel(name string) string {
	for _, tool := range toolConfigs {
		if tool.Name == name {
			return tool.Label
		}
	}
	return name
}

func syncMarkdownDir(sourceDir, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "SKILL.md" {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		src := filepath.Join(sourceDir, entry.Name())
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			same, err := filesEqual(src, dest)
			if err != nil {
				return err
			}
			if same {
				fmt.Printf("  = %s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ %s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + %s\n", entry.Name())
		}
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - %s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func setupTool(projectRoot, tool string) error {
	switch tool {
	case "claude":
		fmt.Println("Linking Claude Code...")
		skillsTarget := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		agentsTarget := filepath.Join(projectRoot, ".agents", "belmont")
		linkAgents := filepath.Join(projectRoot, ".claude", "agents", "belmont")
		if err := ensureSymlink(linkAgents, agentsTarget, true); err != nil {
			return err
		}
		claudeCommandsDir := filepath.Join(projectRoot, ".claude", "commands", "belmont")
		if err := syncMarkdownDir(skillsTarget, claudeCommandsDir); err != nil {
			return err
		}
		claudeSkillsDir := filepath.Join(projectRoot, ".claude", "skills", "belmont")
		if dirExists(claudeSkillsDir) {
			fmt.Println("  - .claude/skills/belmont (deprecated, removing)")
			if err := os.RemoveAll(claudeSkillsDir); err != nil {
				return err
			}
		}
	case "codex":
		fmt.Println("Linking Codex...")
		skillsTarget := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		codexDir := filepath.Join(projectRoot, ".codex", "belmont")
		if err := syncMarkdownDir(skillsTarget, codexDir); err != nil {
			return err
		}
	case "windsurf":
		fmt.Println("Linking Windsurf...")
		target := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		link := filepath.Join(projectRoot, ".windsurf", "rules", "belmont")
		if err := ensureSymlink(link, target, true); err != nil {
			return err
		}
	case "gemini":
		fmt.Println("Linking Gemini...")
		target := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		link := filepath.Join(projectRoot, ".gemini", "rules", "belmont")
		if err := ensureSymlink(link, target, true); err != nil {
			return err
		}
	case "copilot":
		fmt.Println("Linking GitHub Copilot...")
		target := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		link := filepath.Join(projectRoot, ".copilot", "belmont")
		if err := ensureSymlink(link, target, true); err != nil {
			return err
		}
	case "cursor":
		fmt.Println("Linking Cursor...")
		cursorDir := filepath.Join(projectRoot, ".cursor", "rules", "belmont")
		if err := linkPerFileDir(filepath.Join(projectRoot, ".agents", "skills", "belmont"), cursorDir, ".md", ".mdc"); err != nil {
			return err
		}
	}
	return nil
}

func ensureSymlink(linkPath, target string, isDir bool) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return err
	}
	if existing, err := os.Lstat(linkPath); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			current, err := os.Readlink(linkPath)
			if err == nil && current == target {
				fmt.Printf("  = %s (symlink ok)\n", linkPath)
				return nil
			}
		}
		if existing.IsDir() {
			fmt.Printf("  ~ %s (replacing old directory with symlink)\n", linkPath)
			if err := os.RemoveAll(linkPath); err != nil {
				return err
			}
		} else {
			fmt.Printf("  ~ %s (replacing existing file with symlink)\n", linkPath)
			if err := os.Remove(linkPath); err != nil {
				return err
			}
		}
	}

	if err := os.Symlink(target, linkPath); err != nil {
		fmt.Printf("  ! symlink failed for %s (copying instead)\n", linkPath)
		if isDir {
			return copyDir(target, linkPath)
		}
		return copyFile(target, linkPath)
	}
	fmt.Printf("  + %s -> %s\n", linkPath, target)
	return nil
}

func linkPerFileDir(sourceDir, targetDir, sourceExt, targetExt string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	sourceEntries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	sourceNames := make(map[string]struct{})
	for _, entry := range sourceEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), sourceExt) {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), sourceExt)
		sourceNames[name] = struct{}{}
		target := filepath.Join(sourceDir, entry.Name())
		link := filepath.Join(targetDir, name+targetExt)
		if err := ensureSymlink(link, target, false); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), targetExt) {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), targetExt)
		if _, ok := sourceNames[name]; !ok {
			fmt.Printf("  - %s%s (removed, no longer in source)\n", name, targetExt)
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func ensureStateFiles(projectRoot string) error {
	stateDir := filepath.Join(projectRoot, ".belmont")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}

	// Create features directory
	featuresDir := filepath.Join(stateDir, "features")
	if !dirExists(featuresDir) {
		if err := os.MkdirAll(featuresDir, 0o755); err != nil {
			return err
		}
		fmt.Println("  + .belmont/features/")
	} else {
		fmt.Println("  Exists: .belmont/features/ (keeping)")
	}

	// Create PR_FAQ.md template
	prfaqPath := filepath.Join(stateDir, "PR_FAQ.md")
	if !fileExists(prfaqPath) {
		if err := os.WriteFile(prfaqPath, []byte("Run /belmont:working-backwards to create your PR/FAQ document.\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("  + .belmont/PR_FAQ.md")
	} else {
		fmt.Println("  Exists: .belmont/PR_FAQ.md (keeping)")
	}

	prdPath := filepath.Join(stateDir, "PRD.md")
	if !fileExists(prdPath) {
		if err := os.WriteFile(prdPath, []byte("Run the /belmont:product-plan skill to create a plan for your feature.\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("  + .belmont/PRD.md")
	} else {
		fmt.Println("  Exists: .belmont/PRD.md (keeping)")
	}

	return nil
}

const codexAgentsGuidanceStart = "<!-- belmont:codex-skill-routing:start -->"
const codexAgentsGuidanceEnd = "<!-- belmont:codex-skill-routing:end -->"

func ensureCodexAgentsGuidance(projectRoot string) (bool, error) {
	path := filepath.Join(projectRoot, "AGENTS.md")
	current := ""
	if fileExists(path) {
		data, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		current = string(data)
	}

	section := codexAgentsGuidanceSection()
	updated, changed := upsertMarkedSection(current, codexAgentsGuidanceStart, codexAgentsGuidanceEnd, section)
	if !changed {
		return false, nil
	}

	if strings.TrimSpace(updated) == "" {
		updated = "# AGENTS\n\n" + strings.TrimSpace(section) + "\n"
	}
	return true, os.WriteFile(path, []byte(updated), 0o644)
}

func codexAgentsGuidanceSection() string {
	lines := []string{
		codexAgentsGuidanceStart,
		"## Belmont Skill Routing (Codex)",
		"",
		"- Belmont skills are local markdown files in `.agents/skills/belmont/` (and mirrored in `.codex/belmont/`).",
		"- If the user says `belmont:<skill>` or \"Use the belmont:<skill> skill\", treat it as a skill reference, not a shell command.",
		"- Load `.agents/skills/belmont/<skill>.md` first (fallback to `.codex/belmont/<skill>.md`) and follow that workflow.",
		"- Known Belmont skills: `working-backwards`, `product-plan`, `tech-plan`, `implement`, `next`, `verify`, `debug`, `debug-auto`, `debug-manual`, `status`, `reset`, `note`.",
		"- If a requested skill file is missing, list available files in those directories and continue with the closest matching Belmont skill.",
		codexAgentsGuidanceEnd,
	}
	return strings.Join(lines, "\n")
}

func upsertMarkedSection(content, startMarker, endMarker, section string) (string, bool) {
	newSection := strings.TrimSpace(section)
	if strings.TrimSpace(content) == "" {
		return newSection + "\n", true
	}

	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)
	if start >= 0 && end > start {
		end += len(endMarker)
		currentSection := strings.TrimSpace(content[start:end])
		if currentSection == newSection {
			return content, false
		}
		replacement := newSection
		if !strings.HasSuffix(replacement, "\n") {
			replacement += "\n"
		}
		updated := content[:start] + replacement + content[end:]
		return updated, updated != content
	}

	trimmed := strings.TrimRight(content, "\n")
	return trimmed + "\n\n" + newSection + "\n", true
}

func removeLegacyCodexSkillsIndex(projectRoot string) (bool, error) {
	path := filepath.Join(projectRoot, "SKILLS.md")
	if !fileExists(path) {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	content := string(data)
	if !strings.Contains(content, "name: belmont-skills-index") {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}


func filesEqual(a, b string) (bool, error) {
	ab, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return string(ab) == string(bb), nil
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(dest); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(dest); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

func ensureDir(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}
	return os.MkdirAll(path, 0o755)
}

func copyDir(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func containsTool(tools []string, name string) bool {
	for _, tool := range tools {
		if tool == name {
			return true
		}
	}
	return false
}

// syncEmbeddedDir mirrors syncMarkdownDir but reads from an embed.FS.
func syncEmbeddedDir(embedFS embed.FS, root string, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := fs.ReadDir(embedFS, root)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "SKILL.md" {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		data, err := fs.ReadFile(embedFS, root+"/"+entry.Name())
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			existing, err := os.ReadFile(dest)
			if err != nil {
				return err
			}
			if string(existing) == string(data) {
				fmt.Printf("  = %s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ %s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + %s\n", entry.Name())
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}

	// Clean stale files
	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - %s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- Update command ---

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Body    string        `json:"body"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func runUpdate(args []string) error {
	fsFlags := flag.NewFlagSet("update", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var check bool
	var force bool
	fsFlags.BoolVar(&check, "check", false, "check for updates without installing")
	fsFlags.BoolVar(&force, "force", false, "force update even if same version")
	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("update: %w", err)
	}

	if Version == "dev" {
		return errors.New("update: development build detected — use git pull && scripts/build.sh to update")
	}

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	if !force && !isNewer(release.TagName, "v"+Version) {
		fmt.Printf("Already up to date (v%s)\n", Version)
		return nil
	}

	if check {
		fmt.Printf("Update available: v%s → %s\n", Version, release.TagName)
		if release.Body != "" {
			fmt.Println("\nRelease notes:")
			fmt.Println(release.Body)
		}
		return nil
	}

	assetName := fmt.Sprintf("belmont-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("update: no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	if err := checkWriteAccess(filepath.Dir(exePath)); err != nil {
		return fmt.Errorf("update: cannot write to %s — try running with sudo or reinstall to ~/.local/bin", filepath.Dir(exePath))
	}

	fmt.Printf("Downloading %s...\n", assetName)
	tmpPath := exePath + ".tmp"
	if err := downloadFile(downloadURL, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("update: download failed: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Replace self — on Windows, rename current to .old first
	if runtime.GOOS == "windows" {
		oldPath := exePath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(exePath, oldPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("update: %w", err)
		}
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("update: %w", err)
	}

	fmt.Printf("\nUpdated: v%s → %s\n", Version, release.TagName)
	if release.Body != "" {
		fmt.Println("\nRelease notes:")
		fmt.Println(release.Body)
	}

	// Auto-install if .belmont/ exists in cwd
	if dirExists(filepath.Join(".", ".belmont")) {
		fmt.Println("\nRe-installing skills and agents...")
		cmd := exec.Command(exePath, "install", "--no-prompt", "--tools", "all")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Auto-install failed: %v\nRun 'belmont install' manually.\n", err)
		}
	} else {
		fmt.Println("\nTo update skills in a project: cd ~/your-project && belmont install")
	}

	return nil
}

func fetchLatestRelease() (*githubRelease, error) {
	url := "https://api.github.com/repos/blake-simpson/belmont/releases/latest"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach GitHub (are you offline?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		return nil, fmt.Errorf("GitHub API rate limited — set GITHUB_TOKEN env var to authenticate")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func downloadFile(url, dest string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func checkWriteAccess(dir string) error {
	tmp := filepath.Join(dir, ".belmont-update-check")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	f.Close()
	os.Remove(tmp)
	return nil
}

func parseSemver(v string) (int, int, int, bool) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func isNewer(remote, local string) bool {
	rMaj, rMin, rPat, ok1 := parseSemver(remote)
	lMaj, lMin, lPat, ok2 := parseSemver(local)
	if !ok1 || !ok2 {
		return true
	}
	if rMaj != lMaj {
		return rMaj > lMaj
	}
	if rMin != lMin {
		return rMin > lMin
	}
	return rPat > lPat
}

// ── Loop command ──

func runAutoCmd(args []string) error {
	fs := flag.NewFlagSet("auto", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg loopConfig
	var policyStr string
	var featuresFlag string
	var allFlag bool
	fs.StringVar(&cfg.Feature, "feature", "", "feature slug (required)")
	fs.StringVar(&featuresFlag, "features", "", "comma-separated feature slugs for parallel execution")
	fs.BoolVar(&allFlag, "all", false, "run all pending features in parallel")
	fs.StringVar(&cfg.From, "from", "", "start milestone (e.g. M1)")
	fs.StringVar(&cfg.To, "to", "", "end milestone (e.g. M5)")
	fs.StringVar(&cfg.Tool, "tool", "", "CLI tool (claude|codex|gemini|copilot|cursor)")
	fs.StringVar(&policyStr, "policy", "autonomous", "checkpoint policy (autonomous|milestone|every_action)")
	fs.IntVar(&cfg.MaxIterations, "max-iterations", 20, "maximum loop iterations")
	fs.IntVar(&cfg.MaxFailures, "max-failures", 3, "consecutive failures before stopping")
	fs.IntVar(&cfg.MaxParallel, "max-parallel", 3, "max concurrent goroutines for parallel execution")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "show execution plan without running")
	fs.StringVar(&cfg.Root, "root", ".", "project root")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Validate mutual exclusivity
	multiFeature := featuresFlag != "" || allFlag
	if multiFeature && cfg.Feature != "" {
		return fmt.Errorf("auto: --feature cannot be combined with --features or --all")
	}
	if featuresFlag != "" && allFlag {
		return fmt.Errorf("auto: --features and --all are mutually exclusive")
	}
	if multiFeature && (cfg.From != "" || cfg.To != "") {
		return fmt.Errorf("auto: --from/--to cannot be used with --features or --all")
	}
	if !multiFeature && cfg.Feature == "" {
		return fmt.Errorf("auto: --feature is required (or use --features/--all for multi-feature mode)")
	}

	switch checkpointPolicy(policyStr) {
	case policyAutonomous, policyMilestone, policyEveryAction:
		cfg.Policy = checkpointPolicy(policyStr)
	default:
		return fmt.Errorf("auto: invalid --policy %q (use autonomous, milestone, or every_action)", policyStr)
	}

	absRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		return err
	}
	cfg.Root = absRoot

	// Auto-detect tool if not specified
	if cfg.Tool == "" {
		detected := detectTool()
		if detected == "" {
			return fmt.Errorf("auto: no supported AI tool CLI found on PATH\n\nSupported tools: claude, codex, gemini, copilot, cursor\nInstall one or use --tool to specify")
		}
		cfg.Tool = detected
	} else {
		// Validate tool name
		switch cfg.Tool {
		case "claude", "codex", "gemini", "copilot", "cursor":
			// ok
		default:
			return fmt.Errorf("auto: unsupported tool %q (use claude, codex, gemini, copilot, or cursor)", cfg.Tool)
		}
	}

	// Multi-feature mode: --features or --all
	if multiFeature {
		slugs, err := resolveFeatureSlugs(absRoot, featuresFlag, allFlag)
		if err != nil {
			return err
		}
		return runAutoMultiFeature(cfg, slugs)
	}

	// Single-feature mode
	// Verify feature directory exists
	featureDir := filepath.Join(absRoot, ".belmont", "features", cfg.Feature)
	if !dirExists(featureDir) {
		return fmt.Errorf("auto: feature %q not found at %s", cfg.Feature, featureDir)
	}

	// Read milestones and check for dependency syntax
	progressPath := filepath.Join(absRoot, ".belmont", "features", cfg.Feature, "PROGRESS.md")
	progressContent, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("auto: failed to read PROGRESS.md: %w", err)
	}
	milestones := parseMilestones(string(progressContent))
	inRange := milestonesInRange(milestones, cfg.From, cfg.To)

	// Interactive milestone selection when stdin is a terminal and no --from/--to
	if cfg.From == "" && cfg.To == "" && isTerminal(os.Stdin) {
		selectedFrom, selectedTo, err := interactiveMilestoneSelect(inRange)
		if err != nil {
			return err
		}
		cfg.From = selectedFrom
		cfg.To = selectedTo
	}

	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (single-feature) — %s\033[0m\n", cfg.Feature)
		fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Policy: %s\033[0m\n", cfg.Tool, cfg.Policy)
		if cfg.From != "" || cfg.To != "" {
			fmt.Fprintf(os.Stderr, "\033[2mRange: %s → %s\033[0m\n", cfg.From, cfg.To)
		}
		fmt.Fprintf(os.Stderr, "\n\033[1mMilestones:\033[0m\n")
		for _, m := range inRange {
			status := "pending"
			if m.Done {
				status = "done"
			}
			fmt.Fprintf(os.Stderr, "  • %s — %s [%s]\n", m.ID, m.Name, status)
		}
		fmt.Fprintln(os.Stderr)
		return nil
	}

	// Check if any milestones have explicit dependencies
	hasExplicitDeps := false
	for _, m := range inRange {
		if len(m.Deps) > 0 {
			hasExplicitDeps = true
			break
		}
	}

	if hasExplicitDeps {
		return runAutoParallel(cfg, inRange)
	}

	return runLoop(cfg)
}

// resolveFeatureSlugs resolves the list of feature slugs for multi-feature mode.
func resolveFeatureSlugs(root, featuresFlag string, allFlag bool) ([]string, error) {
	featuresDir := filepath.Join(root, ".belmont", "features")

	if allFlag {
		// Get all features, filter to pending ones
		features := listFeatures(featuresDir, 50)
		if len(features) == 0 {
			return nil, fmt.Errorf("auto: no features found in %s", featuresDir)
		}
		var slugs []string
		for _, f := range features {
			if f.Status != "✅ Complete" {
				slugs = append(slugs, f.Slug)
			}
		}
		if len(slugs) == 0 {
			return nil, fmt.Errorf("auto: all features are already complete")
		}
		return slugs, nil
	}

	// Parse comma-separated slugs
	parts := strings.Split(featuresFlag, ",")
	var slugs []string
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		// Verify feature directory exists
		featureDir := filepath.Join(featuresDir, s)
		if !dirExists(featureDir) {
			return nil, fmt.Errorf("auto: feature %q not found at %s", s, featureDir)
		}
		slugs = append(slugs, s)
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("auto: no valid feature slugs provided")
	}
	return slugs, nil
}

// featureWave represents a group of features that can execute in parallel.
type featureWave struct {
	Index    int
	Features []featureSummary
}

// computeFeatureWaves groups features into waves using Kahn's algorithm for topological sort.
// Features in the same wave have all deps satisfied by prior waves.
// Already-complete features satisfy deps but don't execute.
func computeFeatureWaves(features []featureSummary) ([]featureWave, error) {
	if len(features) == 0 {
		return nil, nil
	}

	// Build slug -> feature map
	bySlug := make(map[string]featureSummary)
	for _, f := range features {
		bySlug[f.Slug] = f
	}

	// Compute in-degree for each non-complete feature
	inDegree := make(map[string]int)
	for _, f := range features {
		if f.Status == "✅ Complete" {
			continue
		}
		count := 0
		for _, dep := range f.Deps {
			if df, ok := bySlug[dep]; ok && df.Status != "✅ Complete" {
				count++
			}
		}
		inDegree[f.Slug] = count
	}

	var waves []featureWave
	remaining := len(inDegree)
	waveIdx := 0

	for remaining > 0 {
		// Find all features with zero in-degree
		var ready []featureSummary
		for slug, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, bySlug[slug])
			}
		}

		if len(ready) == 0 {
			var cycleIDs []string
			for slug := range inDegree {
				cycleIDs = append(cycleIDs, slug)
			}
			sort.Strings(cycleIDs)
			return nil, fmt.Errorf("dependency cycle detected among features: %s", strings.Join(cycleIDs, ", "))
		}

		// Sort ready features by slug for deterministic ordering
		sort.Slice(ready, func(i, j int) bool {
			return ready[i].Slug < ready[j].Slug
		})

		waves = append(waves, featureWave{Index: waveIdx, Features: ready})
		waveIdx++

		// Remove completed features and update in-degrees
		for _, f := range ready {
			delete(inDegree, f.Slug)
			remaining--
		}
		for slug, deg := range inDegree {
			f := bySlug[slug]
			newDeg := deg
			for _, dep := range f.Deps {
				for _, completed := range ready {
					if dep == completed.Slug {
						newDeg--
					}
				}
			}
			inDegree[slug] = newDeg
		}
	}

	return waves, nil
}

// validateFeatureDeps checks for dangling dependency references and cycles.
func validateFeatureDeps(features []featureSummary, allKnown []featureSummary) error {
	// Build slug set from ALL known features so completed deps are recognized
	slugSet := make(map[string]bool)
	for _, f := range allKnown {
		slugSet[f.Slug] = true
	}

	// Check for dangling references — collect all errors
	var errs []string
	for _, f := range features {
		for _, dep := range f.Deps {
			if !slugSet[dep] {
				errs = append(errs, fmt.Sprintf("feature %q depends on %q which does not exist", f.Slug, dep))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	// Check for cycles by attempting wave computation
	_, err := computeFeatureWaves(features)
	return err
}

// runAutoMultiFeature orchestrates wave-based execution of multiple features.
// Features with dependencies execute after their dependencies complete.
func runAutoMultiFeature(cfg loopConfig, slugs []string) error {
	startTime := time.Now()

	ensureWorktreesGitignore(cfg.Root)

	// Build feature summaries with dependency info
	featuresDir := filepath.Join(cfg.Root, ".belmont", "features")
	allFeatures := listFeatures(featuresDir, 50)
	populateFeatureDeps(allFeatures, cfg.Root)

	// Filter to requested slugs
	slugSet := make(map[string]bool)
	for _, s := range slugs {
		slugSet[s] = true
	}
	var features []featureSummary
	for _, f := range allFeatures {
		if slugSet[f.Slug] {
			features = append(features, f)
		}
	}

	// Validate dependencies
	if err := validateFeatureDeps(features, allFeatures); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Check if any features have deps — if not, use flat parallel (original behavior)
	hasAnyDeps := false
	for _, f := range features {
		if len(f.Deps) > 0 {
			hasAnyDeps = true
			break
		}
	}

	// Compute waves
	waves, err := computeFeatureWaves(features)
	if err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	if !hasAnyDeps {
		// No dependencies — single wave with all features (original behavior)
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (multi-feature) — %d features\033[0m\n", len(slugs))
	} else {
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (multi-feature) — %d features in %d waves\033[0m\n", len(slugs), len(waves))
	}
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Max parallel: %d\033[0m\n", cfg.Tool, cfg.MaxParallel)

	// Print wave execution plan
	fmt.Fprintf(os.Stderr, "\n\033[1mExecution plan:\033[0m\n")
	for _, w := range waves {
		var names []string
		for _, f := range w.Features {
			names = append(names, f.Slug)
		}
		if len(waves) == 1 {
			for _, n := range names {
				fmt.Fprintf(os.Stderr, "  • %s\n", n)
			}
		} else {
			fmt.Fprintf(os.Stderr, "  Wave %d: [%s]\n", w.Index+1, strings.Join(names, ", "))
		}
	}
	fmt.Fprintln(os.Stderr)

	if cfg.DryRun {
		return nil
	}

	// Set up worktree tracker and signal handler
	activeWorktrees := &worktreeTracker{entries: make(map[string]worktreeEntry)}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Interrupted — cleaning up worktrees...\033[0m\n")
		activeWorktrees.cleanupAll(cfg.Root)
		os.Exit(1)
	}()

	type featureResult struct {
		Slug         string
		Branch       string
		WorktreePath string
		Err          error
	}

	var allFailures []featureResult
	failedSlugs := make(map[string]bool)
	totalMerged := 0

	// Execute wave by wave
	for _, w := range waves {
		// Filter out features whose deps include a failed slug
		var waveFeatures []featureSummary
		var skippedFeatures []string
		for _, f := range w.Features {
			skip := false
			for _, dep := range f.Deps {
				if failedSlugs[dep] {
					skip = true
					break
				}
			}
			if skip {
				skippedFeatures = append(skippedFeatures, f.Slug)
				failedSlugs[f.Slug] = true
			} else {
				waveFeatures = append(waveFeatures, f)
			}
		}

		for _, slug := range skippedFeatures {
			fmt.Fprintf(os.Stderr, "\033[33m⊘ %s skipped\033[0m — dependency failed\n", slug)
			allFailures = append(allFailures, featureResult{Slug: slug, Err: fmt.Errorf("dependency failed")})
		}

		if len(waveFeatures) == 0 {
			continue
		}

		if len(waves) > 1 {
			fmt.Fprintf(os.Stderr, "\n\033[1m── Wave %d ──\033[0m\n", w.Index+1)
		}

		// Run this wave's features in parallel
		semaphore := make(chan struct{}, cfg.MaxParallel)
		var wg sync.WaitGroup
		results := make(chan featureResult, len(waveFeatures))

		for _, f := range waveFeatures {
			wg.Add(1)
			go func(slug string) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				branch := fmt.Sprintf("belmont/auto/%s", slug)
				wtPath := filepath.Join(cfg.Root, ".belmont", "worktrees", slug)

				activeWorktrees.add(slug, wtPath, branch)

				fmt.Fprintf(os.Stderr, "\033[36m▶ %s\033[0m — starting in worktree\n", slug)

				err := runFeatureInWorktree(cfg, slug, branch, wtPath)
				results <- featureResult{
					Slug:         slug,
					Branch:       branch,
					WorktreePath: wtPath,
					Err:          err,
				}
			}(f.Slug)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect wave results
		var waveSuccesses []featureResult
		for r := range results {
			if r.Err != nil {
				fmt.Fprintf(os.Stderr, "\033[31m✗ %s failed: %s\033[0m\n", r.Slug, r.Err)
				allFailures = append(allFailures, r)
				failedSlugs[r.Slug] = true
			} else {
				fmt.Fprintf(os.Stderr, "\033[32m✓ %s complete\033[0m — merging...\n", r.Slug)
				waveSuccesses = append(waveSuccesses, r)
			}
		}

		// Merge this wave's successes before proceeding to next wave
		for _, s := range waveSuccesses {
			if err := mergeFeatureBranch(cfg, s.Slug, s.Branch, s.WorktreePath, activeWorktrees); err != nil {
				fmt.Fprintf(os.Stderr, "\033[31m✗ merge failed for %s: %s\033[0m\n", s.Slug, err)
				fmt.Fprintf(os.Stderr, "  Worktree preserved at: %s\n", s.WorktreePath)
				fmt.Fprintf(os.Stderr, "  Branch: %s\n", s.Branch)
				allFailures = append(allFailures, featureResult{Slug: s.Slug, Err: err})
				failedSlugs[s.Slug] = true
			} else {
				totalMerged++
			}
		}
	}

	// Report
	if len(allFailures) > 0 {
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ %d feature(s) failed:\033[0m\n", len(allFailures))
		for _, f := range allFailures {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", f.Slug, f.Err)
			if f.WorktreePath != "" {
				fmt.Fprintf(os.Stderr, "    Worktree: %s\n", f.WorktreePath)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "\n\033[32m✓ %d/%d features complete\033[0m (%.1fs total)\n", totalMerged, len(slugs), time.Since(startTime).Seconds())

	if len(allFailures) > 0 {
		return fmt.Errorf("auto: %d feature(s) failed", len(allFailures))
	}
	return nil
}

// handleStaleWorktree checks for a stale branch/worktree from a previous interrupted run.
// Returns resumed=true if the existing worktree should be reused (skip creation).
// Returns resumed=false if stale state was cleaned up (proceed with fresh creation).
func handleStaleWorktree(root, id, branch, wtPath string) (resumed bool, err error) {
	// Check if branch already exists
	checkCmd := exec.Command("git", "branch", "--list", branch)
	checkCmd.Dir = root
	out, err := checkCmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return false, nil // no stale branch, proceed normally
	}

	// Stale branch exists — determine what to do
	_, wtDirErr := os.Stat(wtPath)
	wtExists := wtDirErr == nil

	if isTerminal(os.Stdin) {
		// Interactive: prompt the user
		status := "branch exists"
		if wtExists {
			status = "branch + worktree exist"
		}
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Branch '%s' exists from a previous run (%s).\033[0m\n", branch, status)
		fmt.Fprintf(os.Stderr, "  [r] Resume from where it left off\n")
		fmt.Fprintf(os.Stderr, "  [s] Start fresh (delete branch and restart)\n")
		fmt.Fprintf(os.Stderr, "  [q] Quit\n")
		fmt.Fprintf(os.Stderr, "> ")

		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(strings.ToLower(line))

		switch choice {
		case "r", "resume":
			if wtExists {
				// Worktree still exists — reuse it directly
				fmt.Fprintf(os.Stderr, "  Resuming with existing worktree at %s\n", wtPath)
				return true, nil
			}
			// Branch exists but worktree is gone — reattach
			fmt.Fprintf(os.Stderr, "  Reattaching worktree to existing branch %s\n", branch)
			wtDir := filepath.Dir(wtPath)
			if err := os.MkdirAll(wtDir, 0755); err != nil {
				return false, fmt.Errorf("create worktree dir: %w", err)
			}
			addCmd := exec.Command("git", "worktree", "add", wtPath, branch)
			addCmd.Dir = root
			if out, err := addCmd.CombinedOutput(); err != nil {
				return false, fmt.Errorf("git worktree add (resume): %w (%s)", err, strings.TrimSpace(string(out)))
			}
			return true, nil

		case "q", "quit":
			return false, fmt.Errorf("user chose to quit")

		default: // "s", "start", or anything else → start fresh
			fmt.Fprintf(os.Stderr, "  Cleaning up stale state for %s...\n", id)
		}
	} else {
		// Non-interactive: auto-restart
		fmt.Fprintf(os.Stderr, "  Cleaning up stale branch '%s' from previous run...\n", branch)
	}

	// Clean up stale state (restart path)
	if wtExists {
		removeWorktree(root, wtPath, id)
	}
	// Prune any orphaned worktree references
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = root
	pruneCmd.Run()
	// Delete the stale branch
	delCmd := exec.Command("git", "branch", "-D", branch)
	delCmd.Dir = root
	delCmd.Run()

	return false, nil
}

// runFeatureInWorktree creates a worktree for a feature, installs belmont, and runs the full loop.
func runFeatureInWorktree(cfg loopConfig, slug, branch, wtPath string) error {
	// Handle stale worktree/branch from previous interrupted run
	resumed, err := handleStaleWorktree(cfg.Root, slug, branch, wtPath)
	if err != nil {
		return err
	}

	if !resumed {
		// Create worktree directory
		wtDir := filepath.Dir(wtPath)
		if err := os.MkdirAll(wtDir, 0755); err != nil {
			return fmt.Errorf("create worktree dir: %w", err)
		}

		// Create git worktree
		cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath, "HEAD")
		cmd.Dir = cfg.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree add: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}

	// Copy .belmont/features/<slug>/ state to worktree
	srcFeatureDir := filepath.Join(cfg.Root, ".belmont", "features", slug)
	dstFeatureDir := filepath.Join(wtPath, ".belmont", "features", slug)
	if err := os.MkdirAll(dstFeatureDir, 0755); err != nil {
		return fmt.Errorf("create feature dir in worktree: %w", err)
	}
	if err := copyDir(srcFeatureDir, dstFeatureDir); err != nil {
		return fmt.Errorf("copy feature state: %w", err)
	}

	// Ensure .belmont/ is gitignored in the worktree to prevent AI tools from committing state files
	ensureBelmontGitignore(wtPath)

	// Run belmont install in the worktree
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}
	installCmd := exec.Command(exePath, "install", "--project", wtPath, "--no-prompt")
	installCmd.Dir = wtPath
	if out, err := installCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33mInstall warning for %s: %s\033[0m\n", slug, strings.TrimSpace(string(out)))
	}

	// Run loop for this feature (all milestones)
	mCfg := cfg
	mCfg.Root = wtPath
	mCfg.Feature = slug

	return runLoop(mCfg)
}

// mergeFeatureBranch merges a feature branch back to main and cleans up.
func mergeFeatureBranch(cfg loopConfig, slug, branch, wtPath string, tracker *worktreeTracker) error {
	commitMsg := fmt.Sprintf("belmont: merge feature %s", slug)

	if err := attemptMerge(cfg, commitMsg, branch, slug); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[31m✗ Merge failed for feature %s\033[0m\n", slug)
		fmt.Fprintf(os.Stderr, "    Worktree preserved at: %s\n", wtPath)
		fmt.Fprintf(os.Stderr, "    Branch: %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Resolve manually: git merge --no-ff %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Or use: belmont recover --merge %s\n", slug)
		return err
	}

	// Clean up reconciliation report if it exists
	os.Remove(filepath.Join(cfg.Root, ".belmont", "reconciliation-report.json"))

	// Clean up worktree and branch
	removeWorktree(cfg.Root, wtPath, slug)
	tracker.remove(slug)

	// Delete the branch
	delCmd := exec.Command("git", "branch", "-d", branch)
	delCmd.Dir = cfg.Root
	delCmd.Run() // best-effort

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Feature %s merged successfully\033[0m\n", slug)
	return nil
}

func detectTool() string {
	for _, tool := range []string{"claude", "codex", "gemini", "copilot", "cursor"} {
		if _, err := exec.LookPath(tool); err == nil {
			return tool
		}
	}
	return ""
}

func runLoop(cfg loopConfig) error {
	startTime := time.Now()
	var history []historyEntry
	var lastOutput string

	fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto — %s\033[0m\n", cfg.Feature)
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Policy: %s | Max iterations: %d\033[0m\n", cfg.Tool, cfg.Policy, cfg.MaxIterations)
	if cfg.From != "" || cfg.To != "" {
		fromStr := cfg.From
		if fromStr == "" {
			fromStr = "start"
		}
		toStr := cfg.To
		if toStr == "" {
			toStr = "end"
		}
		fmt.Fprintf(os.Stderr, "\033[2mRange: %s → %s\033[0m\n", fromStr, toStr)
	}
	fmt.Fprintln(os.Stderr)

	for i := 1; i <= cfg.MaxIterations; i++ {
		// 1. Read current state
		report, err := buildStatus(cfg.Root, 55, cfg.Feature)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31mFailed to read state: %s\033[0m\n", err)
			return fmt.Errorf("auto: state read failed: %w", err)
		}

		// 2. Derive extra signals
		hasFwlup := detectFwlupTasks(cfg.Root, cfg.Feature, report)
		msStates := buildMilestoneLoopStates(history, report.Milestones)

		// Print state summary
		printLoopState(report, hasFwlup)

		// 3. Check hard guardrails first
		action := checkHardGuardrails(report, history, cfg)

		// 4. If no guardrail triggered, try smart rules first
		if action == nil {
			action = decideLoopActionSmart(report, history, cfg, hasFwlup, msStates)
		}

		// 5. If smart rules returned nil, use AI decisions (with rules fallback)
		if action == nil {
			aiAction, err := decideLoopActionAI(report, history, cfg, hasFwlup, lastOutput, msStates)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\033[33m  AI decision failed: %s — falling back to rules\033[0m\n", err)
				decided := decideLoopAction(report, history, cfg, hasFwlup)
				action = &decided
			} else {
				action = aiAction
			}
		}

		label := describeMilestone(action, report)
		actionLabel := shortActionLabel(action.Type)
		if label != "" {
			fmt.Fprintf(os.Stderr, "\n\033[1m━━ [%d] %s ━━ %s ━━\033[0m\n", i, actionLabel, label)
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[1m━━ [%d] %s ━━\033[0m\n", i, actionLabel)
		}
		fmt.Fprintf(os.Stderr, "\033[2m  %s\033[0m\n\n", action.Reason)

		// 5. Terminal actions
		if action.Type == actionComplete {
			fmt.Fprintf(os.Stderr, "\n\033[32m✓ Complete\033[0m — %s (%.1fs total)\n", action.Reason, time.Since(startTime).Seconds())
			return nil
		}
		if action.Type == actionError {
			fmt.Fprintf(os.Stderr, "\n\033[31m✗ Error\033[0m — %s\n", action.Reason)
			return fmt.Errorf("auto: %s", action.Reason)
		}
		if action.Type == actionPause {
			fmt.Fprintf(os.Stderr, "\n\033[33m⏸ Paused\033[0m — %s\n", action.Reason)
			fmt.Fprintf(os.Stderr, "Resume with: belmont auto --feature %s", cfg.Feature)
			if cfg.From != "" {
				fmt.Fprintf(os.Stderr, " --from %s", cfg.From)
			}
			if cfg.To != "" {
				fmt.Fprintf(os.Stderr, " --to %s", cfg.To)
			}
			fmt.Fprintln(os.Stderr)
			return nil
		}

		// 6. Handle SKIP_MILESTONE (state mutation, no tool call)
		if action.Type == actionSkipMilestone {
			skipErr := skipMilestoneInProgress(cfg.Root, cfg.Feature, action.MilestoneID)
			if skipErr != nil {
				fmt.Fprintf(os.Stderr, "\033[31m  ✗ Failed to skip milestone: %s\033[0m\n\n", skipErr)
			} else {
				fmt.Fprintf(os.Stderr, "\033[32m  ✓ Skipped milestone %s\033[0m\n\n", action.MilestoneID)
			}
			entry := historyEntry{
				Action:       *action,
				Result:       &executionResult{Success: skipErr == nil, DurationMs: 0},
				TasksDone:    report.TaskCounts["done"],
				TasksTotal:   report.TaskCounts["total"],
				MsDone:       countDoneMilestones(report.Milestones),
				MsTotal:      len(report.Milestones),
				BlockerCount: len(report.Blockers),
				HasFwlup:     hasFwlup,
				Iteration:    i,
			}
			history = append(history, entry)
			continue
		}

		// 7. Checkpoint policy check
		if shouldLoopCheckpoint(*action, cfg.Policy, lastActionType(history)) {
			fmt.Fprintf(os.Stderr, "\n\033[33m⏸ Checkpoint\033[0m — %s\n", action.Reason)
			fmt.Fprintf(os.Stderr, "Resume with: belmont auto --feature %s", cfg.Feature)
			if cfg.From != "" {
				fmt.Fprintf(os.Stderr, " --from %s", cfg.From)
			}
			if cfg.To != "" {
				fmt.Fprintf(os.Stderr, " --to %s", cfg.To)
			}
			fmt.Fprintln(os.Stderr)
			return nil
		}

		// 8. Capture pre-action SHA
		preSHA := captureGitSHA(cfg.Root)

		// 9. Execute action
		result := executeLoopAction(*action, cfg)
		lastOutput = truncateTail(result.Output, 1500)

		// 10. Post-action classification
		postSHA := captureGitSHA(cfg.Root)
		wt, fc := classifyChanges(cfg.Root, preSHA)

		// 11. Record in history
		entry := historyEntry{
			Action:       *action,
			Result:       &result,
			TasksDone:    report.TaskCounts["done"],
			TasksTotal:   report.TaskCounts["total"],
			MsDone:       countDoneMilestones(report.Milestones),
			MsTotal:      len(report.Milestones),
			BlockerCount: len(report.Blockers),
			HasFwlup:     hasFwlup,
			Iteration:    i,
			WorkType:     wt,
			FilesChanged: fc,
			GitSHA:       preSHA,
			PostGitSHA:   postSHA,
		}
		history = append(history, entry)

		// 12. Print result
		if result.Success {
			fmt.Fprintf(os.Stderr, "\n\033[32m  ✓ %.1fs\033[0m\n", float64(result.DurationMs)/1000)
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[31m  ✗ %s (%.1fs)\033[0m\n", result.Error, float64(result.DurationMs)/1000)
		}
	}

	fmt.Fprintf(os.Stderr, "\n\033[33m⏸ Max iterations reached (%d)\033[0m\n", cfg.MaxIterations)
	return nil
}

func printLoopState(report statusReport, hasFwlup bool) {
	done := report.TaskCounts["done"]
	total := report.TaskCounts["total"]
	msDone := countDoneMilestones(report.Milestones)
	msTotal := len(report.Milestones)

	// Progress bar
	barWidth := 20
	filled := 0
	if total > 0 {
		filled = (done * barWidth) / total
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	fmt.Fprintf(os.Stderr, "  [%s] %d/%d tasks, %d/%d milestones", bar, done, total, msDone, msTotal)

	if hasFwlup {
		fmt.Fprintf(os.Stderr, " \033[33m(FWLUP)\033[0m")
	}
	if len(report.Blockers) > 0 {
		fmt.Fprintf(os.Stderr, " \033[31m(%d blockers)\033[0m", len(report.Blockers))
	}
	fmt.Fprintln(os.Stderr)
}

func decideLoopAction(report statusReport, history []historyEntry, cfg loopConfig, hasFwlup bool) loopAction {
	last := lastActionType(history)

	// Rule 1: Blockers → PAUSE
	if len(report.Blockers) > 0 {
		return loopAction{Type: actionPause, Reason: fmt.Sprintf("Blockers detected: %s", strings.Join(report.Blockers, ", "))}
	}

	// Rule 2: Consecutive failures >= maxFailures → ERROR
	if consecutiveFailures(history) >= cfg.MaxFailures {
		return loopAction{Type: actionError, Reason: fmt.Sprintf("%d consecutive failures", cfg.MaxFailures)}
	}

	// Rule 3: Stuck detection
	if isLoopStuck(history) {
		return loopAction{Type: actionPause, Reason: "Loop appears stuck — no state change after 2 iterations"}
	}

	// Rule 4: FWLUP tasks after VERIFY → IMPLEMENT_NEXT
	if hasFwlup && last == actionImplementNext {
		// After implementing next (follow-up fix), re-verify
		return loopAction{Type: actionVerify, Reason: "Re-verifying after follow-up fix"}
	}

	if hasFwlup && last == actionVerify {
		return loopAction{Type: actionImplementNext, Reason: "Follow-up tasks detected after verification"}
	}

	// Rule 5: After IMPLEMENT_NEXT → VERIFY
	if last == actionImplementNext {
		return loopAction{Type: actionVerify, Reason: "Re-verifying after follow-up fix"}
	}

	// Rule 6: After IMPLEMENT_MILESTONE → VERIFY
	if last == actionImplementMilestone {
		return loopAction{Type: actionVerify, Reason: "Verifying completed milestone"}
	}

	// Rule 7-10: Check milestones in range
	inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
	allDone := true
	for _, m := range inRange {
		if !m.Done {
			allDone = false
			break
		}
	}

	// Rule 7: All done + no FWLUP → COMPLETE
	if allDone && !hasFwlup {
		return loopAction{Type: actionComplete, Reason: "All milestones in range completed"}
	}

	// Rule 8: All done but FWLUP remaining → IMPLEMENT_NEXT
	if allDone && hasFwlup {
		return loopAction{Type: actionImplementNext, Reason: "Follow-up tasks remaining after all milestones complete"}
	}

	// Rule 10: Next milestone in range → IMPLEMENT_MILESTONE
	for _, m := range inRange {
		if !m.Done {
			return loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Implementing milestone %s", m.ID), MilestoneID: m.ID}
		}
	}

	// Fallback
	return loopAction{Type: actionComplete, Reason: "No actionable milestones found"}
}

func milestonesInRange(milestones []milestone, from, to string) []milestone {
	if from == "" && to == "" {
		return milestones
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	var result []milestone
	for _, m := range milestones {
		num := parseMilestoneNum(m.ID)
		if num < 0 {
			continue
		}
		if fromNum >= 0 && num < fromNum {
			continue
		}
		if toNum >= 0 && num > toNum {
			continue
		}
		result = append(result, m)
	}
	return result
}

func parseMilestoneNum(id string) int {
	if id == "" {
		return -1
	}
	re := regexp.MustCompile(`(?i)M(\d+)`)
	match := re.FindStringSubmatch(id)
	if len(match) < 2 {
		return -1
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return n
}

// tailWriter writes all data to an underlying writer and keeps a rolling
// buffer of the last `size` bytes for later retrieval.
type tailWriter struct {
	out  io.Writer
	buf  []byte
	size int
}

func newTailWriter(out io.Writer, size int) *tailWriter {
	return &tailWriter{out: out, buf: make([]byte, 0, size), size: size}
}

func (tw *tailWriter) Write(p []byte) (int, error) {
	n, err := tw.out.Write(p)
	if n > 0 {
		tw.buf = append(tw.buf, p[:n]...)
		if len(tw.buf) > tw.size {
			tw.buf = tw.buf[len(tw.buf)-tw.size:]
		}
	}
	return n, err
}

func (tw *tailWriter) String() string {
	return string(tw.buf)
}

// claudeStreamWriter wraps a tailWriter and parses Claude stream-json NDJSON,
// extracting only human-readable content (assistant text + tool use indicators).
type claudeStreamWriter struct {
	tw      *tailWriter
	partial []byte
}

type streamLine struct {
	Type    string        `json:"type"`
	Message streamMessage `json:"message"`
}

type streamMessage struct {
	Content []streamContent `json:"content"`
}

type streamContent struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

func (c *claudeStreamWriter) Write(p []byte) (int, error) {
	c.partial = append(c.partial, p...)
	for {
		idx := bytes.IndexByte(c.partial, '\n')
		if idx < 0 {
			break
		}
		line := c.partial[:idx]
		c.partial = c.partial[idx+1:]
		if len(line) == 0 {
			continue
		}
		var sl streamLine
		if err := json.Unmarshal(line, &sl); err != nil {
			continue
		}
		if sl.Type != "assistant" {
			continue
		}
		for _, item := range sl.Message.Content {
			switch item.Type {
			case "text":
				if item.Text != "" {
					c.tw.Write([]byte("  " + item.Text + "\n"))
				}
			case "tool_use":
				if item.Name != "" {
					c.tw.Write([]byte("  → " + toolSummary(item.Name, item.Input) + "\n"))
				}
			}
		}
	}
	return len(p), nil
}

func toolSummary(name string, input map[string]interface{}) string {
	switch name {
	case "Read", "Write", "Edit":
		if fp, ok := input["file_path"].(string); ok {
			return name + " " + filepath.Base(fp)
		}
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 60 {
				cmd = cmd[:60] + "…"
			}
			return name + " " + cmd
		}
	case "Grep":
		if pat, ok := input["pattern"].(string); ok {
			return name + " " + strconv.Quote(pat)
		}
	case "Glob":
		if pat, ok := input["pattern"].(string); ok {
			return name + " " + pat
		}
	case "Agent":
		if desc, ok := input["description"].(string); ok {
			if len(desc) > 60 {
				desc = desc[:60] + "…"
			}
			return name + " " + desc
		}
	case "Skill":
		if sk, ok := input["skill"].(string); ok {
			return name + " " + sk
		}
	}
	return name
}

func shortActionLabel(t loopActionType) string {
	switch t {
	case actionImplementMilestone:
		return "IMPLEMENT"
	case actionImplementNext:
		return "FIX"
	case actionVerify:
		return "VERIFY"
	case actionReplan:
		return "REPLAN"
	case actionSkipMilestone:
		return "SKIP"
	case actionDebug:
		return "DEBUG"
	default:
		return string(t)
	}
}

func describeMilestone(action *loopAction, report statusReport) string {
	if action.MilestoneID != "" {
		for _, m := range report.Milestones {
			if m.ID == action.MilestoneID {
				return m.ID + ": " + m.Name
			}
		}
	}
	if action.Type == actionVerify || action.Type == actionImplementNext {
		if report.NextMilestone != nil {
			return report.NextMilestone.ID + ": " + report.NextMilestone.Name
		}
	}
	return ""
}

func executeLoopAction(action loopAction, cfg loopConfig) executionResult {
	prompt := buildLoopPrompt(action, cfg.Feature)

	var cmd *exec.Cmd
	switch cfg.Tool {
	case "claude":
		cmd = exec.Command("claude", "-p", prompt,
			"--permission-mode", "bypassPermissions",
			"--allowedTools", "Bash Read Write Edit Glob Grep Agent Skill",
			"--output-format", "stream-json", "--verbose")
	case "codex":
		cmd = exec.Command("codex", "exec", prompt,
			"--dangerously-bypass-approvals-and-sandbox",
			"--json", "-C", cfg.Root)
	case "gemini":
		cmd = exec.Command("gemini", prompt,
			"--yolo", "--output-format", "json")
	case "copilot":
		cmd = exec.Command("copilot", "-p", prompt, "--yolo")
	case "cursor":
		cmd = exec.Command("cursor", "agent", "-p", prompt,
			"--force", "--output-format", "json")
	default:
		return executionResult{Success: false, Error: fmt.Sprintf("unsupported tool: %s", cfg.Tool)}
	}

	cmd.Dir = cfg.Root

	tw := newTailWriter(os.Stderr, 1500)
	if cfg.Tool == "claude" {
		cmd.Stdout = &claudeStreamWriter{tw: tw}
	} else {
		cmd.Stdout = tw
	}
	cmd.Stderr = tw

	var stopTimer chan struct{}
	if cfg.Tool != "claude" {
		stopTimer = make(chan struct{})
		go func() {
			start := time.Now()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					fmt.Fprintf(os.Stderr, "\r\033[2m  ⏱ %s\033[0m", time.Since(start).Truncate(time.Second))
				case <-stopTimer:
					fmt.Fprintf(os.Stderr, "\r\033[K")
					return
				}
			}
		}()
	}

	start := time.Now()
	err := cmd.Run()
	if stopTimer != nil {
		close(stopTimer)
	}
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		return executionResult{
			Success:    false,
			Output:     tw.String(),
			Error:      err.Error(),
			DurationMs: durationMs,
		}
	}

	return executionResult{
		Success:    true,
		Output:     tw.String(),
		DurationMs: durationMs,
	}
}

func buildLoopPrompt(action loopAction, feature string) string {
	switch action.Type {
	case actionImplementMilestone:
		return fmt.Sprintf("/belmont:implement --feature %s", feature)
	case actionImplementNext:
		return fmt.Sprintf("/belmont:next --feature %s", feature)
	case actionVerify:
		return fmt.Sprintf("/belmont:verify --feature %s", feature)
	case actionReplan:
		return fmt.Sprintf("/belmont:tech-plan --feature %s", feature)
	case actionDebug:
		return fmt.Sprintf("/belmont:debug-auto --feature %s", feature)
	default:
		return ""
	}
}

func shouldLoopCheckpoint(action loopAction, policy checkpointPolicy, last loopActionType) bool {
	// Terminal actions handled elsewhere
	if action.Type == actionPause || action.Type == actionError || action.Type == actionComplete {
		return false
	}

	switch policy {
	case policyAutonomous:
		return false
	case policyMilestone:
		if action.Type == actionImplementMilestone {
			return true
		}
		// Significant changes warrant a checkpoint
		if action.Type == actionReplan || action.Type == actionSkipMilestone {
			return true
		}
		// Auto-verify after implement
		if action.Type == actionVerify && last == actionImplementMilestone {
			return false
		}
		// Pause after verify results
		if last == actionVerify {
			return true
		}
		return false
	case policyEveryAction:
		return true
	}
	return false
}

func lastActionType(history []historyEntry) loopActionType {
	if len(history) == 0 {
		return ""
	}
	return history[len(history)-1].Action.Type
}

func consecutiveFailures(history []historyEntry) int {
	count := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Result != nil && !history[i].Result.Success {
			count++
		} else {
			break
		}
	}
	return count
}

func isLoopStuck(history []historyEntry) bool {
	if len(history) < 2 {
		return false
	}
	recent := history[len(history)-2:]
	// Both must have succeeded
	for _, e := range recent {
		if e.Result == nil || !e.Result.Success {
			return false
		}
	}
	// Compare state fingerprints
	fp0 := loopFingerprint(recent[0])
	fp1 := loopFingerprint(recent[1])
	return fp0 == fp1
}

func loopFingerprint(e historyEntry) string {
	return fmt.Sprintf("%d/%d|%d/%d|%d|%v|%s", e.TasksDone, e.TasksTotal, e.MsDone, e.MsTotal, e.BlockerCount, e.HasFwlup, e.PostGitSHA)
}

func countDoneMilestones(milestones []milestone) int {
	count := 0
	for _, m := range milestones {
		if m.Done {
			count++
		}
	}
	return count
}

// captureGitSHA returns the current HEAD SHA, or "" on error.
func captureGitSHA(root string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// classifyChanges runs git diff between preSHA and HEAD and classifies the work type.
func classifyChanges(root, preSHA string) (workType, int) {
	if preSHA == "" {
		return workUnknown, 0
	}
	cmd := exec.Command("git", "diff", "--name-only", preSHA+"..HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return workUnknown, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			files = append(files, l)
		}
	}
	if len(files) == 0 {
		return workMinimal, 0
	}
	if len(files) < 3 {
		return workMinimal, len(files)
	}

	frontendExts := map[string]bool{
		".tsx": true, ".jsx": true, ".css": true, ".scss": true,
		".html": true, ".vue": true, ".svelte": true, ".less": true,
	}
	backendExts := map[string]bool{
		".go": true, ".py": true, ".rs": true, ".java": true,
		".rb": true, ".php": true, ".cs": true, ".kt": true,
		".scala": true, ".ex": true, ".exs": true,
	}
	configExts := map[string]bool{
		".yml": true, ".yaml": true, ".json": true, ".toml": true,
		".ini": true, ".env": true,
	}
	docExts := map[string]bool{
		".md": true, ".txt": true, ".rst": true,
	}

	var feCount, beCount, cfgCount, docCount int
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		switch {
		case frontendExts[ext]:
			feCount++
		case backendExts[ext]:
			beCount++
		case configExts[ext]:
			cfgCount++
		case docExts[ext]:
			docCount++
		}
	}

	total := len(files)
	if docCount == total {
		return workDocs, total
	}
	if cfgCount == total {
		return workConfig, total
	}
	if feCount*2 > total {
		return workFrontend, total
	}
	if beCount*2 > total {
		return workBackend, total
	}
	if feCount > 0 && beCount > 0 {
		return workMixed, total
	}
	if feCount > 0 {
		return workFrontend, total
	}
	if beCount > 0 {
		return workBackend, total
	}
	return workMixed, total
}

// isCriticalConfig returns true if changed files include runtime-affecting config.
func isCriticalConfig(files []string) bool {
	for _, f := range files {
		base := strings.ToLower(filepath.Base(f))
		ext := strings.ToLower(filepath.Ext(f))
		if ext == ".css" || ext == ".scss" || ext == ".less" {
			return true
		}
		if strings.Contains(base, ".env") || strings.Contains(base, "styles") {
			return true
		}
		if base == "tailwind.config.js" || base == "tailwind.config.ts" ||
			base == "postcss.config.js" || base == "vite.config.ts" ||
			base == "next.config.js" || base == "next.config.mjs" {
			return true
		}
	}
	return false
}

// buildMilestoneLoopStates derives per-milestone state from the loop history.
func buildMilestoneLoopStates(history []historyEntry, milestones []milestone) map[string]*milestoneLoopState {
	states := make(map[string]*milestoneLoopState)
	for _, m := range milestones {
		states[m.ID] = &milestoneLoopState{
			ID:   m.ID,
			Name: m.Name,
			Done: m.Done,
		}
	}

	var lastImplementedMS string
	for _, h := range history {
		switch h.Action.Type {
		case actionImplementMilestone:
			msID := h.Action.MilestoneID
			if msID != "" {
				if s, ok := states[msID]; ok && h.Result != nil && h.Result.Success {
					s.Implemented = true
					s.WorkType = h.WorkType
					s.FilesChanged = h.FilesChanged
					lastImplementedMS = msID
				}
			}
		case actionVerify:
			// Attribute verification to the most recently implemented milestone
			if lastImplementedMS != "" {
				if s, ok := states[lastImplementedMS]; ok {
					if h.Result != nil {
						if h.Result.Success {
							s.Verified = true
						} else {
							s.VerifyFailed++
						}
					}
				}
			}
		}
	}
	return states
}

// decideLoopActionSmart applies deterministic rules for ~80% of cases.
// Returns nil for ambiguous cases that should fall through to AI.
func decideLoopActionSmart(report statusReport, history []historyEntry, cfg loopConfig, hasFwlup bool, msStates map[string]*milestoneLoopState) *loopAction {
	if len(history) == 0 {
		// First iteration: implement first undone milestone in range
		inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
		for _, m := range inRange {
			if !m.Done {
				return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("First iteration — implementing %s", m.ID), MilestoneID: m.ID}
			}
		}
		// All done already
		if !hasFwlup {
			return &loopAction{Type: actionComplete, Reason: "All milestones already complete"}
		}
		return &loopAction{Type: actionImplementNext, Reason: "All milestones done but follow-up tasks remain"}
	}

	last := history[len(history)-1]
	lastType := last.Action.Type
	lastSuccess := last.Result != nil && last.Result.Success

	// Rule 1: After IMPLEMENT_MILESTONE success → almost always VERIFY
	if lastType == actionImplementMilestone && lastSuccess {
		wt := last.WorkType
		fc := last.FilesChanged
		// Skip verify only for: 0 files changed, pure docs, or non-critical config ≤2 files
		if fc == 0 {
			return &loopAction{Type: actionImplementNext, Reason: "No files changed — skipping verification"}
		}
		if wt == workDocs {
			// Docs-only: skip verification, move to next milestone
			inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
			for _, m := range inRange {
				if !m.Done {
					return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Docs-only milestone — moving to %s", m.ID), MilestoneID: m.ID}
				}
			}
			if !hasFwlup {
				return &loopAction{Type: actionComplete, Reason: "All milestones complete (last was docs-only)"}
			}
			return &loopAction{Type: actionImplementNext, Reason: "Docs-only milestone done, fixing follow-ups"}
		}
		// Everything else: verify
		return &loopAction{Type: actionVerify, Reason: "Verifying completed milestone"}
	}

	// Rule 2: After VERIFY success + no follow-ups → next undone milestone or COMPLETE
	if lastType == actionVerify && lastSuccess && !hasFwlup {
		inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
		for _, m := range inRange {
			if !m.Done {
				return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Verification passed — implementing %s", m.ID), MilestoneID: m.ID}
			}
		}
		return &loopAction{Type: actionComplete, Reason: "All milestones verified and complete"}
	}

	// Rule 3: After VERIFY success + follow-ups exist → IMPLEMENT_NEXT
	if lastType == actionVerify && lastSuccess && hasFwlup {
		return &loopAction{Type: actionImplementNext, Reason: "Follow-up tasks detected after verification"}
	}

	// Rule 4: After IMPLEMENT_NEXT success → VERIFY (re-verify)
	if lastType == actionImplementNext && lastSuccess {
		return &loopAction{Type: actionVerify, Reason: "Re-verifying after follow-up fix"}
	}

	// Rule 5: After VERIFY failure — check verify failure count
	if lastType == actionVerify && !lastSuccess {
		// Find the milestone being verified
		var verifyFailCount int
		var targetMS string
		// Walk history backward to find which milestone we're verifying
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Action.Type == actionImplementMilestone && history[i].Action.MilestoneID != "" {
				targetMS = history[i].Action.MilestoneID
				break
			}
		}
		if targetMS != "" {
			if s, ok := msStates[targetMS]; ok {
				verifyFailCount = s.VerifyFailed
			}
		}

		if verifyFailCount >= 2 {
			// Delegate to AI for REPLAN/DEBUG decision
			return nil
		}
		// First failure: try fixing
		return &loopAction{Type: actionImplementNext, Reason: "Verification failed — fixing issues"}
	}

	// Rule 6: After DEBUG success → VERIFY
	if lastType == actionDebug && lastSuccess {
		return &loopAction{Type: actionVerify, Reason: "Re-verifying after debug"}
	}

	// Rule 7: All milestones done + verified + no follow-ups → COMPLETE
	inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
	allDone := true
	allVerified := true
	for _, m := range inRange {
		if !m.Done {
			allDone = false
			break
		}
		if s, ok := msStates[m.ID]; ok {
			if !s.Verified {
				allVerified = false
			}
		}
	}
	if allDone && allVerified && !hasFwlup {
		return &loopAction{Type: actionComplete, Reason: "All milestones implemented, verified, and no follow-ups"}
	}

	// Rule 8: All done but not all verified → VERIFY
	if allDone && !allVerified && !hasFwlup {
		return &loopAction{Type: actionVerify, Reason: "All milestones done but not all verified"}
	}

	// Rule 9: All done but follow-ups remain → IMPLEMENT_NEXT
	if allDone && hasFwlup {
		return &loopAction{Type: actionImplementNext, Reason: "Follow-up tasks remaining after all milestones complete"}
	}

	// Rule 10: Next undone milestone
	for _, m := range inRange {
		if !m.Done {
			return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Implementing milestone %s", m.ID), MilestoneID: m.ID}
		}
	}

	// Ambiguous — let AI decide
	return nil
}

// checkHardGuardrails runs safety checks that always apply before AI decisions.
// Returns nil if no guardrail triggers, otherwise a loopAction to take.
func checkHardGuardrails(report statusReport, history []historyEntry, cfg loopConfig) *loopAction {
	// Blockers → PAUSE
	if len(report.Blockers) > 0 {
		return &loopAction{Type: actionPause, Reason: fmt.Sprintf("Blockers detected: %s", strings.Join(report.Blockers, ", "))}
	}

	// Consecutive failures >= maxFailures → ERROR
	if consecutiveFailures(history) >= cfg.MaxFailures {
		return &loopAction{Type: actionError, Reason: fmt.Sprintf("%d consecutive failures", cfg.MaxFailures)}
	}

	// Stuck detection
	if isLoopStuck(history) {
		return &loopAction{Type: actionPause, Reason: "Loop appears stuck — no state change after 2 iterations"}
	}

	return nil
}

// decideLoopActionAI shells out to the configured tool to make a strategic decision.
// Only called for ambiguous cases that decideLoopActionSmart couldn't handle.
func decideLoopActionAI(report statusReport, history []historyEntry, cfg loopConfig, hasFwlup bool, lastOutput string, msStates map[string]*milestoneLoopState) (*loopAction, error) {
	// Build rich milestone state JSON
	inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
	type msStateJSON struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Done           bool   `json:"done"`
		Implemented    bool   `json:"implemented"`
		Verified       bool   `json:"verified"`
		VerifyFailures int    `json:"verify_failures,omitempty"`
		WorkType       string `json:"work_type,omitempty"`
		FilesChanged   int    `json:"files_changed,omitempty"`
	}
	var milestones []msStateJSON
	for _, m := range inRange {
		ms := msStateJSON{ID: m.ID, Name: m.Name, Done: m.Done}
		if s, ok := msStates[m.ID]; ok {
			ms.Implemented = s.Implemented
			ms.Verified = s.Verified
			ms.VerifyFailures = s.VerifyFailed
			ms.WorkType = string(s.WorkType)
			ms.FilesChanged = s.FilesChanged
		}
		milestones = append(milestones, ms)
	}

	// Build recent history (last 5)
	type histItem struct {
		Action    string `json:"action"`
		Milestone string `json:"milestone,omitempty"`
		Success   bool   `json:"success"`
		WorkType  string `json:"work_type,omitempty"`
		Output    string `json:"output,omitempty"`
	}
	var recentHistory []histItem
	start := len(history) - 5
	if start < 0 {
		start = 0
	}
	for _, h := range history[start:] {
		item := histItem{
			Action:    string(h.Action.Type),
			Milestone: h.Action.MilestoneID,
			WorkType:  string(h.WorkType),
		}
		if h.Result != nil {
			item.Success = h.Result.Success
			item.Output = truncateTail(h.Result.Output, 500)
		}
		recentHistory = append(recentHistory, item)
	}

	// Determine why we're asking the AI (ambiguity reason)
	ambiguityReason := "Smart rules could not determine the next action"
	if len(history) > 0 {
		last := history[len(history)-1]
		if last.Action.Type == actionVerify && last.Result != nil && !last.Result.Success {
			// Find which milestone
			var targetMS string
			for i := len(history) - 1; i >= 0; i-- {
				if history[i].Action.Type == actionImplementMilestone && history[i].Action.MilestoneID != "" {
					targetMS = history[i].Action.MilestoneID
					break
				}
			}
			if targetMS != "" {
				if s, ok := msStates[targetMS]; ok && s.VerifyFailed >= 2 {
					ambiguityReason = fmt.Sprintf("%s failed verification %d times — consider REPLAN or DEBUG", targetMS, s.VerifyFailed)
				}
			}
		}
	}

	state := map[string]interface{}{
		"tasks_done":       report.TaskCounts["done"],
		"tasks_total":      report.TaskCounts["total"],
		"milestone_states": milestones,
		"has_followup":     hasFwlup,
		"blocker_count":    len(report.Blockers),
		"last_5_actions":   recentHistory,
		"ambiguity_reason": ambiguityReason,
	}
	if cfg.From != "" {
		state["milestone_from"] = cfg.From
	}
	if cfg.To != "" {
		state["milestone_to"] = cfg.To
	}
	if lastOutput != "" {
		state["previous_output"] = truncateTail(lastOutput, 1500)
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal state: %w", err)
	}

	// Load prompt template
	tmpl, tmplErr := loadPromptTemplate("ai-decision")
	var prompt string
	if tmplErr != nil {
		// Fallback to inline prompt if template not found
		prompt = fmt.Sprintf(`You are a loop controller for an automated feature implementation system.
You are ONLY called for ambiguous cases — simple decisions are already handled by deterministic rules.

STATE:
%s

AVAILABLE ACTIONS:
- IMPLEMENT_MILESTONE: Implement next incomplete milestone (set milestone_id)
- IMPLEMENT_NEXT: Fix follow-up tasks or issues found during verification
- VERIFY: Run verification on completed milestones
- REPLAN: Re-run tech planning when current approach has systemic issues
- DEBUG: Run automated debugging when verification keeps failing on recurring issues
- SKIP_MILESTONE: Skip a blocked milestone (set milestone_id)
- COMPLETE: All work in scope is done and verified
- PAUSE: Stop for human intervention

HARD RULES:
1. You are ONLY called for ambiguous cases — simple decisions are already handled.
2. Never skip verification for frontend/UI milestones.
3. If verification failed 2+ times on the SAME issue, choose REPLAN or DEBUG.
4. If verification failed on DIFFERENT issues each time, one more VERIFY is reasonable.
5. If a milestone has recurring failures across multiple cycles, use DEBUG.
6. Use SKIP_MILESTONE only when a milestone truly cannot proceed due to external blockers.
7. If all milestones in range are done+verified with no follow-ups, COMPLETE.

Respond with ONLY valid JSON: {"action":"...","reason":"...","milestone_id":"..."}`, string(stateJSON))
	} else {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, map[string]string{"StateJSON": string(stateJSON)}); err != nil {
			return nil, fmt.Errorf("execute prompt template: %w", err)
		}
		prompt = buf.String()
	}

	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tool execution: %w (output: %s)", err, truncateTail(string(output), 200))
	}

	decisionJSON, err := extractDecisionJSON(string(output), cfg.Tool)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var decision aiDecision
	if err := json.Unmarshal([]byte(decisionJSON), &decision); err != nil {
		return nil, fmt.Errorf("unmarshal decision: %w", err)
	}

	// Validate action type
	actionType := loopActionType(decision.Action)
	switch actionType {
	case actionImplementMilestone, actionImplementNext, actionVerify,
		actionReplan, actionSkipMilestone, actionComplete, actionPause, actionDebug:
		// valid
	default:
		return nil, fmt.Errorf("unknown action %q from AI", decision.Action)
	}

	// Validate milestone_id required for certain actions
	if (actionType == actionImplementMilestone || actionType == actionSkipMilestone) && decision.MilestoneID == "" {
		return nil, fmt.Errorf("action %s requires milestone_id", actionType)
	}

	return &loopAction{
		Type:        actionType,
		Reason:      decision.Reason,
		MilestoneID: decision.MilestoneID,
	}, nil
}

// buildToolCommand creates an exec.Cmd for the given tool with a prompt.
// Used by both AI decision calls and action execution.
func buildToolCommand(tool, prompt, root string) *exec.Cmd {
	var cmd *exec.Cmd
	switch tool {
	case "claude":
		cmd = exec.Command("claude", "-p", prompt,
			"--permission-mode", "bypassPermissions",
			"--output-format", "json")
	case "codex":
		cmd = exec.Command("codex", "exec", prompt,
			"--dangerously-bypass-approvals-and-sandbox",
			"--json", "-C", root)
	case "gemini":
		cmd = exec.Command("gemini", prompt,
			"--yolo", "--output-format", "json")
	case "copilot":
		cmd = exec.Command("copilot", "-p", prompt, "--yolo")
	case "cursor":
		cmd = exec.Command("cursor", "agent", "-p", prompt,
			"--force", "--output-format", "json")
	default:
		cmd = exec.Command("echo", "unsupported tool")
	}
	cmd.Dir = root
	return cmd
}

// extractDecisionJSON finds the JSON object in tool output.
// Tools may wrap output in their own JSON structure.
func extractDecisionJSON(output, tool string) (string, error) {
	// For tools that return JSON wrapper (claude, codex, gemini, cursor),
	// try to extract the text content first
	text := output

	// Try to extract result text from claude's JSON wrapper
	if tool == "claude" || tool == "codex" || tool == "gemini" || tool == "cursor" {
		var wrapper struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal([]byte(output), &wrapper); err == nil && wrapper.Result != "" {
			text = wrapper.Result
		}
	}

	// Find the JSON object with action field
	re := regexp.MustCompile(`\{[^{}]*"action"\s*:\s*"[^"]+?"[^{}]*\}`)
	match := re.FindString(text)
	if match == "" {
		return "", fmt.Errorf("no decision JSON found in output: %s", truncateTail(text, 200))
	}
	return match, nil
}

// truncateTail returns the last maxLen characters of s.
func truncateTail(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}

// skipMilestoneInProgress marks a milestone as done in PROGRESS.md.
func skipMilestoneInProgress(root, feature, milestoneID string) error {
	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("read PROGRESS.md: %w", err)
	}

	// Replace [ ] with [x] for the matching milestone line
	re := regexp.MustCompile(`(?im)(^- \[ \]\s+` + regexp.QuoteMeta(milestoneID) + `\b.*)`)
	updated := re.ReplaceAllString(string(content), "- [x] "+milestoneID+" (skipped)")

	if updated == string(content) {
		return fmt.Errorf("milestone %s not found or already done", milestoneID)
	}

	return os.WriteFile(progressPath, []byte(updated), 0644)
}

func detectFwlupTasks(root, feature string, report statusReport) bool {
	prdPath := filepath.Join(root, ".belmont", "features", feature, "PRD.md")
	prdContent, err := os.ReadFile(prdPath)
	if err != nil {
		return false
	}

	prd := string(prdContent)
	fwlupRe := regexp.MustCompile(`(?i)FWLUP`)
	if !fwlupRe.MatchString(prd) {
		return false
	}

	// Check if any pending/in_progress tasks have FWLUP in their ID or name
	for _, t := range report.Tasks {
		if t.Status == taskPending || t.Status == taskInProgress {
			if fwlupRe.MatchString(t.ID) || fwlupRe.MatchString(t.Name) {
				return true
			}
		}
	}
	return false
}

// loadPromptTemplate loads a prompt template from embedded FS or source filesystem.
func loadPromptTemplate(name string) (*template.Template, error) {
	filename := name + ".md"

	// Try embedded first
	if hasEmbeddedFiles {
		data, err := fs.ReadFile(embeddedPrompts, filepath.Join("prompts", "belmont", filename))
		if err == nil {
			return template.New(name).Parse(string(data))
		}
	}

	// Try source resolution
	sourceRoot := resolveSourceForPrompts()
	if sourceRoot == "" {
		return nil, fmt.Errorf("prompt %q: no embedded files and no source directory found", name)
	}

	data, err := os.ReadFile(filepath.Join(sourceRoot, "prompts", "belmont", filename))
	if err != nil {
		return nil, fmt.Errorf("prompt %q: %w", name, err)
	}
	return template.New(name).Parse(string(data))
}

// resolveSourceForPrompts returns the belmont source directory path, or "" if not found.
func resolveSourceForPrompts() string {
	if src := os.Getenv("BELMONT_SOURCE"); src != "" {
		return src
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		configPath := filepath.Join(configDir, "belmont", "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			var cfg config
			if json.Unmarshal(data, &cfg) == nil && cfg.Source != "" {
				return cfg.Source
			}
		}
	}

	// Walk up from binary location
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for {
		if _, err := os.Stat(filepath.Join(dir, "prompts", "belmont")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// isTerminal returns true if the given file is a terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// interactiveMilestoneSelect shows milestones and lets user pick a range.
func interactiveMilestoneSelect(milestones []milestone) (from, to string, err error) {
	if len(milestones) == 0 {
		return "", "", nil
	}

	fmt.Fprintf(os.Stderr, "\033[1mMilestones:\033[0m\n")
	firstUndone := ""
	for _, m := range milestones {
		marker := "⬜"
		if m.Done {
			marker = "✅"
		}
		if !m.Done && firstUndone == "" {
			firstUndone = m.ID
		}
		depStr := ""
		if len(m.Deps) > 0 {
			depStr = fmt.Sprintf(" \033[2m(depends: %s)\033[0m", strings.Join(m.Deps, ", "))
		}
		fmt.Fprintf(os.Stderr, "  %s %s: %s%s\n", marker, m.ID, m.Name, depStr)
	}

	lastID := milestones[len(milestones)-1].ID
	defaultRange := ""
	if firstUndone != "" {
		defaultRange = fmt.Sprintf("%s → %s", firstUndone, lastID)
	}

	fmt.Fprintf(os.Stderr, "\n\033[2mDefault range: %s\033[0m\n", defaultRange)
	fmt.Fprintf(os.Stderr, "Press Enter to accept, 'q' to quit, or enter range (e.g. M2 M5): ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", "", fmt.Errorf("auto: no input")
	}
	input := strings.TrimSpace(scanner.Text())

	if input == "q" || input == "quit" || input == "exit" {
		return "", "", fmt.Errorf("auto: cancelled by user")
	}

	if input == "" {
		// Accept defaults
		return "", "", nil
	}

	// Parse custom range
	parts := strings.Fields(input)
	if len(parts) == 1 {
		return parts[0], parts[0], nil
	}
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("auto: invalid range %q — use 'M2 M5' format", input)
}

// wave represents a group of milestones that can execute in parallel.
type wave struct {
	Index      int
	Milestones []milestone
}

// computeWaves groups milestones into waves using Kahn's algorithm for topological sort.
// Milestones in the same wave have all deps satisfied by prior waves.
// Already-done milestones satisfy deps but don't execute.
func computeWaves(milestones []milestone) ([]wave, error) {
	if len(milestones) == 0 {
		return nil, nil
	}

	// Build ID -> milestone map
	byID := make(map[string]milestone)
	for _, m := range milestones {
		byID[m.ID] = m
	}

	// Compute in-degree for each undone milestone
	inDegree := make(map[string]int)
	for _, m := range milestones {
		if m.Done {
			continue
		}
		count := 0
		for _, dep := range m.Deps {
			if dm, ok := byID[dep]; ok && !dm.Done {
				count++
			}
		}
		inDegree[m.ID] = count
	}

	var waves []wave
	remaining := len(inDegree)
	waveIdx := 0

	for remaining > 0 {
		// Find all milestones with zero in-degree
		var ready []milestone
		for id, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, byID[id])
			}
		}

		if len(ready) == 0 {
			// Cycle detected
			var cycleIDs []string
			for id := range inDegree {
				cycleIDs = append(cycleIDs, id)
			}
			sort.Strings(cycleIDs)
			return nil, fmt.Errorf("dependency cycle detected among milestones: %s", strings.Join(cycleIDs, ", "))
		}

		// Sort ready milestones by ID for deterministic ordering
		sort.Slice(ready, func(i, j int) bool {
			return parseMilestoneNum(ready[i].ID) < parseMilestoneNum(ready[j].ID)
		})

		waves = append(waves, wave{Index: waveIdx, Milestones: ready})
		waveIdx++

		// Remove completed milestones and update in-degrees
		for _, m := range ready {
			delete(inDegree, m.ID)
			remaining--
		}
		for id, deg := range inDegree {
			m := byID[id]
			newDeg := deg
			for _, dep := range m.Deps {
				for _, completed := range ready {
					if dep == completed.ID {
						newDeg--
					}
				}
			}
			inDegree[id] = newDeg
		}
	}

	return waves, nil
}

// runAutoParallel executes milestones with dependency-aware parallel waves using worktrees.
func runAutoParallel(cfg loopConfig, milestones []milestone) error {
	startTime := time.Now()

	// Ensure .belmont/worktrees/ is in .gitignore
	ensureWorktreesGitignore(cfg.Root)

	fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (parallel) — %s\033[0m\n", cfg.Feature)
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Max parallel: %d\033[0m\n", cfg.Tool, cfg.MaxParallel)

	waves, err := computeWaves(milestones)
	if err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	if len(waves) == 0 {
		fmt.Fprintf(os.Stderr, "\n\033[32m✓ Complete\033[0m — all milestones already done\n")
		return nil
	}

	// Print wave plan
	fmt.Fprintf(os.Stderr, "\n\033[1mExecution plan:\033[0m\n")
	for _, w := range waves {
		var ids []string
		for _, m := range w.Milestones {
			ids = append(ids, m.ID)
		}
		parallel := ""
		if len(w.Milestones) > 1 {
			parallel = " (parallel)"
		}
		fmt.Fprintf(os.Stderr, "  Wave %d: %s%s\n", w.Index+1, strings.Join(ids, ", "), parallel)
	}
	fmt.Fprintln(os.Stderr)

	// Set up signal handler for cleanup
	activeWorktrees := &worktreeTracker{entries: make(map[string]worktreeEntry)}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Interrupted — cleaning up worktrees...\033[0m\n")
		activeWorktrees.cleanupAll(cfg.Root)
		os.Exit(1)
	}()

	for _, w := range waves {
		fmt.Fprintf(os.Stderr, "\033[1m━━ Wave %d ━━\033[0m\n", w.Index+1)

		if len(w.Milestones) == 1 {
			// Single milestone: run directly in main tree
			m := w.Milestones[0]
			fmt.Fprintf(os.Stderr, "  Running %s: %s\n", m.ID, m.Name)
			mCfg := cfg
			mCfg.From = m.ID
			mCfg.To = m.ID
			if err := runLoop(mCfg); err != nil {
				return fmt.Errorf("auto: wave %d, %s failed: %w", w.Index+1, m.ID, err)
			}
		} else {
			// Multiple milestones: run in parallel via worktrees
			if err := runWaveParallel(cfg, w, activeWorktrees); err != nil {
				return err
			}
		}

		fmt.Fprintf(os.Stderr, "\033[32m  ✓ Wave %d complete\033[0m\n\n", w.Index+1)
	}

	fmt.Fprintf(os.Stderr, "\n\033[32m✓ All waves complete\033[0m (%.1fs total)\n", time.Since(startTime).Seconds())
	return nil
}

// worktreeEntry stores both the path and branch name for a worktree.
type worktreeEntry struct {
	Path   string
	Branch string
}

// worktreeTracker keeps track of active worktrees for cleanup on interrupt.
type worktreeTracker struct {
	mu      sync.Mutex
	entries map[string]worktreeEntry // ID -> worktree entry
}

func (wt *worktreeTracker) add(id, path, branch string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	wt.entries[id] = worktreeEntry{Path: path, Branch: branch}
}

func (wt *worktreeTracker) remove(id string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	delete(wt.entries, id)
}

func (wt *worktreeTracker) cleanupAll(root string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	for id, entry := range wt.entries {
		fmt.Fprintf(os.Stderr, "  Cleaning up worktree for %s...\n", id)
		removeWorktree(root, entry.Path, id)
		// Also delete the branch to prevent stale branch on restart
		delCmd := exec.Command("git", "branch", "-D", entry.Branch)
		delCmd.Dir = root
		delCmd.Run() // best-effort
	}
	wt.entries = make(map[string]worktreeEntry)
}

// runWaveParallel runs multiple milestones in parallel using git worktrees.
func runWaveParallel(cfg loopConfig, w wave, tracker *worktreeTracker) error {
	semaphore := make(chan struct{}, cfg.MaxParallel)
	var wg sync.WaitGroup

	type result struct {
		MilestoneID  string
		Branch       string
		WorktreePath string
		Err          error
	}
	results := make(chan result, len(w.Milestones))

	for _, m := range w.Milestones {
		wg.Add(1)
		go func(ms milestone) {
			defer wg.Done()
			semaphore <- struct{}{}        // acquire
			defer func() { <-semaphore }() // release

			branch := fmt.Sprintf("belmont/auto/%s/%s", cfg.Feature, strings.ToLower(ms.ID))
			wtPath := filepath.Join(cfg.Root, ".belmont", "worktrees", fmt.Sprintf("%s-%s", cfg.Feature, strings.ToLower(ms.ID)))

			tracker.add(ms.ID, wtPath, branch)

			fmt.Fprintf(os.Stderr, "  \033[36m▶ %s: %s\033[0m (worktree)\n", ms.ID, ms.Name)

			err := runMilestoneInWorktree(cfg, ms, branch, wtPath)
			results <- result{
				MilestoneID:  ms.ID,
				Branch:       branch,
				WorktreePath: wtPath,
				Err:          err,
			}
		}(m)
	}

	// Wait for all goroutines then close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var successes []result
	var failures []result
	for r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  \033[31m✗ %s failed: %s\033[0m\n", r.MilestoneID, r.Err)
			failures = append(failures, r)
		} else {
			fmt.Fprintf(os.Stderr, "  \033[32m✓ %s complete\033[0m\n", r.MilestoneID)
			successes = append(successes, r)
		}
	}

	// Merge successful branches in milestone ID order
	sort.Slice(successes, func(i, j int) bool {
		return parseMilestoneNum(successes[i].MilestoneID) < parseMilestoneNum(successes[j].MilestoneID)
	})

	for _, s := range successes {
		if err := mergeWorktreeBranch(cfg, s.MilestoneID, s.Branch, s.WorktreePath, tracker); err != nil {
			return fmt.Errorf("auto: merge failed for %s: %w", s.MilestoneID, err)
		}
	}

	// Clean up failed worktrees (preserve for manual intervention)
	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ %d milestone(s) failed in wave %d:\033[0m\n", len(failures), w.Index+1)
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  %s: worktree preserved at %s\n", f.MilestoneID, f.WorktreePath)
			fmt.Fprintf(os.Stderr, "    Resume: cd %s && belmont auto --feature %s --from %s --to %s\n", f.WorktreePath, cfg.Feature, f.MilestoneID, f.MilestoneID)
		}
		return fmt.Errorf("auto: wave %d had %d failure(s)", w.Index+1, len(failures))
	}

	return nil
}

// runMilestoneInWorktree creates a worktree, installs belmont, copies state, and runs the loop.
func runMilestoneInWorktree(cfg loopConfig, ms milestone, branch, wtPath string) error {
	// Handle stale worktree/branch from previous interrupted run
	resumed, err := handleStaleWorktree(cfg.Root, ms.ID, branch, wtPath)
	if err != nil {
		return err
	}

	if !resumed {
		// Create worktree directory
		wtDir := filepath.Dir(wtPath)
		if err := os.MkdirAll(wtDir, 0755); err != nil {
			return fmt.Errorf("create worktree dir: %w", err)
		}

		// Create git worktree
		cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath, "HEAD")
		cmd.Dir = cfg.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree add: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}

	// Copy .belmont/features/<slug>/ state to worktree
	srcFeatureDir := filepath.Join(cfg.Root, ".belmont", "features", cfg.Feature)
	dstFeatureDir := filepath.Join(wtPath, ".belmont", "features", cfg.Feature)
	if err := os.MkdirAll(dstFeatureDir, 0755); err != nil {
		return fmt.Errorf("create feature dir in worktree: %w", err)
	}
	if err := copyDir(srcFeatureDir, dstFeatureDir); err != nil {
		return fmt.Errorf("copy feature state: %w", err)
	}

	// Ensure .belmont/ is gitignored in the worktree to prevent AI tools from committing state files
	ensureBelmontGitignore(wtPath)

	// Run belmont install in the worktree (shell out to self)
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}
	installCmd := exec.Command(exePath, "install", "--project", wtPath, "--no-prompt")
	installCmd.Dir = wtPath
	if out, err := installCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "    \033[33mInstall warning for %s: %s\033[0m\n", ms.ID, strings.TrimSpace(string(out)))
	}

	// Run loop for this single milestone
	mCfg := cfg
	mCfg.Root = wtPath
	mCfg.From = ms.ID
	mCfg.To = ms.ID

	return runLoop(mCfg)
}

// mergeWorktreeBranch merges a milestone branch back and cleans up the worktree.
func mergeWorktreeBranch(cfg loopConfig, milestoneID, branch, wtPath string, tracker *worktreeTracker) error {
	// Find the milestone name for the commit message
	var msName string
	progressPath := filepath.Join(cfg.Root, ".belmont", "features", cfg.Feature, "PROGRESS.md")
	if data, err := os.ReadFile(progressPath); err == nil {
		for _, m := range parseMilestones(string(data)) {
			if m.ID == milestoneID {
				msName = m.Name
				break
			}
		}
	}

	commitMsg := fmt.Sprintf("belmont: merge %s (%s)", milestoneID, msName)

	if err := attemptMerge(cfg, commitMsg, branch, milestoneID); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[31m✗ Merge failed for %s\033[0m\n", milestoneID)
		fmt.Fprintf(os.Stderr, "    Worktree preserved at: %s\n", wtPath)
		fmt.Fprintf(os.Stderr, "    Branch: %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Resolve manually: git merge --no-ff %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Or use: belmont recover --merge %s\n", filepath.Base(wtPath))
		return err
	}

	// Clean up reconciliation report if it exists
	os.Remove(filepath.Join(cfg.Root, ".belmont", "reconciliation-report.json"))

	// Clean up worktree and branch
	removeWorktree(cfg.Root, wtPath, milestoneID)
	tracker.remove(milestoneID)

	// Delete the branch
	delCmd := exec.Command("git", "branch", "-d", branch)
	delCmd.Dir = cfg.Root
	delCmd.Run() // best-effort

	return nil
}

// runReconciliationAgent orchestrates two-pass merge conflict resolution.
// Pass 1: AI analyzes conflicts and writes a structured report.
// Pass 2: Go auto-applies high-confidence resolutions and prompts for low-confidence ones.
// Falls back to legacy full-resolve if the report is invalid.
func runReconciliationAgent(cfg loopConfig, milestoneID, branch string) error {
	// Get list of conflicted files
	conflictCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	conflictCmd.Dir = cfg.Root
	conflictOut, err := conflictCmd.Output()
	if err != nil {
		return fmt.Errorf("list conflicts: %w", err)
	}

	conflictedFiles := strings.TrimSpace(string(conflictOut))
	if conflictedFiles == "" {
		return fmt.Errorf("no conflicted files found")
	}

	reportPath := filepath.Join(cfg.Root, ".belmont", "reconciliation-report.json")

	// Pass 1: AI analysis — writes structured report to disk
	if err := runReconciliationAnalysis(cfg, milestoneID, branch, conflictedFiles, reportPath); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Analysis failed, falling back to legacy resolve...\033[0m\n")
		return runLegacyReconciliation(cfg, milestoneID, branch, conflictedFiles)
	}

	// Read and parse the report
	report, err := parseReconciliationReport(reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Invalid report (%v), falling back to legacy resolve...\033[0m\n", err)
		os.Remove(reportPath)
		return runLegacyReconciliation(cfg, milestoneID, branch, conflictedFiles)
	}

	// Pass 2: Apply resolutions
	if err := applyReconciliationReport(cfg, report); err != nil {
		os.Remove(reportPath)
		return err
	}

	// Verify no conflict markers remain
	var resolvedFiles []string
	for _, f := range report.Files {
		resolvedFiles = append(resolvedFiles, f.File)
	}
	if err := verifyNoConflictMarkers(cfg.Root, resolvedFiles); err != nil {
		os.Remove(reportPath)
		return err
	}

	// Clean up report file
	os.Remove(reportPath)
	return nil
}

// runReconciliationAnalysis invokes the AI to analyze conflicts and write a JSON report.
func runReconciliationAnalysis(cfg loopConfig, milestoneID, branch, conflictedFiles, reportPath string) error {
	prompt := fmt.Sprintf(`You are a merge conflict analysis agent. Analyze all merge conflicts and write a structured JSON report.

Conflicted files:
%s

Milestone: %s
Branch: %s

TASK: For each conflicted file, read it, analyze the conflict, and classify your confidence in resolving it.

CONFIDENCE CRITERIA:
- "high": Import merges, non-overlapping function additions, additive changes to different sections, formatting/comment changes
- "low": Same function body modified by both sides, conflicting config values, structural changes to same type/interface, changes with potential semantic interaction

Write a JSON file to: %s

The JSON must have this exact structure:
{
  "files": [
    {
      "file": "path/to/file",
      "confidence": "high" or "low",
      "reason": "Why this confidence level",
      "conflict_summary": "Brief: what Side A did vs what Side B did",
      "resolved_content": "The complete resolved file content (no conflict markers)"
    }
  ]
}

RULES:
1. Combine both sides — never choose one side over the other
2. Include all imports from both sides (remove duplicates)
3. Never delete functionality from either side
4. The resolved_content must be the COMPLETE file with conflicts resolved
5. Do NOT modify any files on disk — only write the JSON report
6. Do NOT run git add — only write the report
7. Include ALL conflicted files in the report`, conflictedFiles, milestoneID, branch, reportPath)

	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// parseReconciliationReport reads and validates the JSON report file.
func parseReconciliationReport(reportPath string) (reconciliationReport, error) {
	var report reconciliationReport

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return report, fmt.Errorf("read report: %w", err)
	}

	if err := json.Unmarshal(data, &report); err != nil {
		return report, fmt.Errorf("parse report: %w", err)
	}

	if len(report.Files) == 0 {
		return report, fmt.Errorf("report contains no files")
	}

	// Validate each file entry
	for i, f := range report.Files {
		if f.File == "" {
			return report, fmt.Errorf("file entry %d missing file path", i)
		}
		if f.Confidence != "high" && f.Confidence != "low" {
			return report, fmt.Errorf("file %q has invalid confidence %q", f.File, f.Confidence)
		}
		if f.ResolvedContent == "" {
			return report, fmt.Errorf("file %q has empty resolved_content", f.File)
		}
	}

	return report, nil
}

// applyReconciliationReport applies resolved content from the report.
// High-confidence files are auto-applied. Low-confidence files are shown
// to the user interactively (if terminal) or auto-applied (if non-interactive).
func applyReconciliationReport(cfg loopConfig, report reconciliationReport) error {
	interactive := isTerminal(os.Stdin)
	autoAll := false

	var highCount, lowCount int
	for _, f := range report.Files {
		if f.Confidence == "high" {
			highCount++
		} else {
			lowCount++
		}
	}

	if highCount > 0 {
		fmt.Fprintf(os.Stderr, "  \033[32m✓ Auto-applying %d high-confidence resolution(s)\033[0m\n", highCount)
	}
	if lowCount > 0 && interactive {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ %d file(s) need review\033[0m\n", lowCount)
	}

	for _, f := range report.Files {
		filePath := filepath.Join(cfg.Root, f.File)

		if f.Confidence == "high" || !interactive || autoAll {
			// Auto-apply
			if err := os.WriteFile(filePath, []byte(f.ResolvedContent), 0644); err != nil {
				return fmt.Errorf("write %s: %w", f.File, err)
			}
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			if err := addCmd.Run(); err != nil {
				return fmt.Errorf("git add %s: %w", f.File, err)
			}
			continue
		}

		// Low confidence + interactive: prompt user
		choice, err := reviewConflict(cfg.Root, f)
		if err != nil {
			return err
		}

		switch choice {
		case "auto":
			autoAll = true
			// Apply this file and all remaining
			if err := os.WriteFile(filePath, []byte(f.ResolvedContent), 0644); err != nil {
				return fmt.Errorf("write %s: %w", f.File, err)
			}
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			if err := addCmd.Run(); err != nil {
				return fmt.Errorf("git add %s: %w", f.File, err)
			}
		case "accept", "edited":
			// File already written by reviewConflict for "edited", write for "accept"
			if choice == "accept" {
				if err := os.WriteFile(filePath, []byte(f.ResolvedContent), 0644); err != nil {
					return fmt.Errorf("write %s: %w", f.File, err)
				}
			}
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			if err := addCmd.Run(); err != nil {
				return fmt.Errorf("git add %s: %w", f.File, err)
			}
		}
	}

	return nil
}

// reviewConflict prompts the user to review a low-confidence conflict resolution.
func reviewConflict(root string, f reconciliationFile) (string, error) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Fprintf(os.Stderr, "\n  \033[1;33m⚠ Uncertain conflict in %s\033[0m\n", f.File)
		fmt.Fprintf(os.Stderr, "    %s\n\n", f.Reason)
		fmt.Fprintf(os.Stderr, "    %s\n\n", f.ConflictSummary)
		fmt.Fprintf(os.Stderr, "    [a] Accept AI's resolution  [v] View proposed resolution\n")
		fmt.Fprintf(os.Stderr, "    [e] Edit in $EDITOR         [s] Auto-resolve all remaining  [q] Abort\n\n")
		fmt.Fprintf(os.Stderr, "    Choice [a]: ")

		if !scanner.Scan() {
			return "", fmt.Errorf("reconciliation: no input")
		}
		input := strings.TrimSpace(strings.ToLower(scanner.Text()))

		if input == "" || input == "a" {
			return "accept", nil
		}

		switch input {
		case "v":
			// Show resolved content with line numbers
			fmt.Fprintf(os.Stderr, "\n    \033[2m--- Proposed resolution for %s ---\033[0m\n", f.File)
			lines := strings.Split(f.ResolvedContent, "\n")
			for i, line := range lines {
				fmt.Fprintf(os.Stderr, "    \033[2m%4d\033[0m  %s\n", i+1, line)
			}
			fmt.Fprintf(os.Stderr, "    \033[2m--- End ---\033[0m\n")
			// Re-prompt
			continue

		case "e":
			// Write proposed resolution to file for editing
			filePath := filepath.Join(root, f.File)
			if err := os.WriteFile(filePath, []byte(f.ResolvedContent), 0644); err != nil {
				return "", fmt.Errorf("write for edit %s: %w", f.File, err)
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			editorCmd := exec.Command(editor, filePath)
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			if err := editorCmd.Run(); err != nil {
				return "", fmt.Errorf("editor for %s: %w", f.File, err)
			}
			return "edited", nil

		case "s":
			return "auto", nil

		case "q":
			return "", fmt.Errorf("reconciliation: aborted by user")

		default:
			fmt.Fprintf(os.Stderr, "    Invalid choice. Try again.\n")
		}
	}
}

// verifyNoConflictMarkers scans resolved files for leftover conflict markers.
func verifyNoConflictMarkers(root string, files []string) error {
	markers := []string{"<<<<<<<", "=======", ">>>>>>>"}
	var badFiles []string

	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			continue
		}
		content := string(data)
		for _, marker := range markers {
			if strings.Contains(content, marker) {
				badFiles = append(badFiles, file)
				break
			}
		}
	}

	if len(badFiles) > 0 {
		return fmt.Errorf("conflict markers remain in: %s", strings.Join(badFiles, ", "))
	}
	return nil
}

// runLegacyReconciliation is the fallback: AI resolves everything directly on disk.
func runLegacyReconciliation(cfg loopConfig, milestoneID, branch, conflictedFiles string) error {
	prompt := fmt.Sprintf(`You are a merge conflict reconciliation agent. Resolve all merge conflicts in the following files:

Conflicted files:
%s

Milestone: %s
Branch: %s

Rules:
1. Combine both sides — never choose one side over the other
2. Include all imports from both sides (remove duplicates)
3. Never delete functionality from either side
4. Only modify conflicted files
5. After resolving each file, run "git add <file>"
6. Do NOT commit — the caller handles the commit

Read each conflicted file, resolve the conflict markers, write the resolved version, and git add it.`, conflictedFiles, milestoneID, branch)

	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// removeWorktree removes a git worktree and its directory.
func removeWorktree(root, wtPath, _ string) {
	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		// Try manual cleanup
		os.RemoveAll(wtPath)
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = root
		pruneCmd.Run()
	}
}

// ensureWorktreesGitignore adds .belmont/worktrees/ to .gitignore if not present.
func ensureWorktreesGitignore(root string) {
	ensureGitignoreEntry(root, ".belmont/worktrees/")
}

// ensureBelmontGitignore adds .belmont/ to .gitignore if not present.
// Used in worktrees to prevent AI tools from committing .belmont/ state files.
func ensureBelmontGitignore(root string) {
	ensureGitignoreEntry(root, ".belmont/")
}

// ensureGitignoreEntry adds an entry to .gitignore if not already present.
func ensureGitignoreEntry(root, entry string) {
	gitignorePath := filepath.Join(root, ".gitignore")

	content, err := os.ReadFile(gitignorePath)
	if err == nil {
		if strings.Contains(string(content), entry) {
			return // already present
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	// Add newline before if file doesn't end with one
	if len(content) > 0 && content[len(content)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(entry + "\n")
}

// mergeFailureKind classifies the type of git merge failure.
type mergeFailureKind int

const (
	mergeConflict            mergeFailureKind = iota // file-level conflicts
	mergeUntrackedOverwrite                          // untracked files would be overwritten
	mergeDirtyWorktree                               // local changes would be overwritten
	mergeOtherFailure                                // unknown merge failure
)

// classifyMergeError determines what kind of merge failure occurred from git output.
func classifyMergeError(output string) mergeFailureKind {
	if strings.Contains(output, "untracked working tree files would be overwritten") {
		return mergeUntrackedOverwrite
	}
	if strings.Contains(output, "local changes would be overwritten") {
		return mergeDirtyWorktree
	}
	if strings.Contains(output, "CONFLICT") || strings.Contains(output, "Automatic merge failed") {
		return mergeConflict
	}
	return mergeOtherFailure
}

// parseOverwrittenFiles extracts file paths from git's "untracked working tree files would be overwritten" error.
func parseOverwrittenFiles(output string) []string {
	var files []string
	lines := strings.Split(output, "\n")
	inFileList := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "untracked working tree files would be overwritten") {
			inFileList = true
			continue
		}
		if inFileList {
			if trimmed == "" || strings.HasPrefix(trimmed, "Please move or remove") || strings.HasPrefix(trimmed, "Aborting") {
				break
			}
			if trimmed != "" {
				files = append(files, trimmed)
			}
		}
	}
	return files
}

// attemptMerge tries to merge a branch, handling various failure modes automatically.
// It handles untracked file overwrites, dirty worktrees, and merge conflicts.
func attemptMerge(cfg loopConfig, commitMsg, branch, id string) error {
	// Try the merge
	cmd := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
	cmd.Dir = cfg.Root
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil // merge succeeded
	}

	output := string(out)
	kind := classifyMergeError(output)

	switch kind {
	case mergeUntrackedOverwrite:
		// Temporarily stash untracked files that conflict
		files := parseOverwrittenFiles(output)
		if len(files) == 0 {
			return fmt.Errorf("merge failed (untracked overwrite) but could not parse file list: %s", output)
		}

		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Untracked files would be overwritten for %s — auto-stashing %d files...\033[0m\n", id, len(files))

		stashDir := filepath.Join(cfg.Root, ".belmont", "merge-stash")
		if err := os.MkdirAll(stashDir, 0755); err != nil {
			return fmt.Errorf("create merge-stash dir: %w", err)
		}

		// Move conflicting files to stash
		for _, f := range files {
			src := filepath.Join(cfg.Root, f)
			dst := filepath.Join(stashDir, f)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				continue
			}
			os.Rename(src, dst)
		}

		// Retry the merge
		retryCmd := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
		retryCmd.Dir = cfg.Root
		retryOut, retryErr := retryCmd.CombinedOutput()

		if retryErr == nil {
			// Merge succeeded — clean up stash
			os.RemoveAll(stashDir)
			fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge succeeded after stashing untracked files for %s\033[0m\n", id)
			return nil
		}

		// Retry failed — restore stashed files and fall through
		for _, f := range files {
			src := filepath.Join(stashDir, f)
			dst := filepath.Join(cfg.Root, f)
			os.MkdirAll(filepath.Dir(dst), 0755)
			os.Rename(src, dst)
		}
		os.RemoveAll(stashDir)

		// Classify the retry failure
		retryOutput := string(retryOut)
		retryKind := classifyMergeError(retryOutput)
		if retryKind == mergeConflict {
			// Fall through to conflict handling below
			goto handleConflict
		}
		return fmt.Errorf("merge failed for %s after stashing untracked files: %s", id, retryOutput)

	case mergeDirtyWorktree:
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Local changes would be overwritten for %s — stashing...\033[0m\n", id)

		stashCmd := exec.Command("git", "stash", "push", "--include-untracked", "-m", "belmont: pre-merge stash")
		stashCmd.Dir = cfg.Root
		if _, stashErr := stashCmd.CombinedOutput(); stashErr != nil {
			return fmt.Errorf("git stash failed for %s: %w", id, stashErr)
		}

		// Retry the merge
		retryCmd := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
		retryCmd.Dir = cfg.Root
		retryOut, retryErr := retryCmd.CombinedOutput()

		// Pop the stash (best-effort)
		popCmd := exec.Command("git", "stash", "pop")
		popCmd.Dir = cfg.Root
		popCmd.Run()

		if retryErr == nil {
			fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge succeeded after stashing local changes for %s\033[0m\n", id)
			return nil
		}

		retryOutput := string(retryOut)
		retryKind := classifyMergeError(retryOutput)
		if retryKind == mergeConflict {
			goto handleConflict
		}
		return fmt.Errorf("merge failed for %s after stashing local changes: %s", id, retryOutput)

	case mergeConflict:
		goto handleConflict

	default:
		return fmt.Errorf("merge failed for %s: %s", id, output)
	}

handleConflict:
	fmt.Fprintf(os.Stderr, "  \033[33m⚠ Merge conflict for %s — invoking reconciliation agent...\033[0m\n", id)

	reconcileErr := runReconciliationAgent(cfg, id, branch)
	if reconcileErr != nil {
		abortCmd := exec.Command("git", "merge", "--abort")
		abortCmd.Dir = cfg.Root
		abortCmd.Run()
		return fmt.Errorf("merge conflict resolution failed for %s: %w", id, reconcileErr)
	}

	// Reconciliation succeeded — commit the merge
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = cfg.Root
	if _, commitErr := commitCmd.CombinedOutput(); commitErr != nil {
		return fmt.Errorf("commit after reconciliation for %s: %w", id, commitErr)
	}

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Reconciliation resolved merge conflict for %s\033[0m\n", id)
	return nil
}

// listPreservedWorktrees finds worktrees under .belmont/worktrees/ that still exist.
func listPreservedWorktrees(root string) []worktreeEntry {
	wtDir := filepath.Join(root, ".belmont", "worktrees")
	entries, err := os.ReadDir(wtDir)
	if err != nil {
		return nil
	}

	var result []worktreeEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wtPath := filepath.Join(wtDir, e.Name())
		// Check if it's actually a git worktree by looking for .git file
		gitFile := filepath.Join(wtPath, ".git")
		if _, err := os.Stat(gitFile); err != nil {
			continue
		}
		// Try to find the branch
		branch := "belmont/" + e.Name()
		result = append(result, worktreeEntry{Path: wtPath, Branch: branch})
	}
	return result
}

// runRecover handles the "belmont recover" command.
func runRecover(args []string) error {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root string
	var format string
	var list bool
	var merge string
	var clean string
	var cleanAll bool
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&format, "format", "text", "text or json")
	fs.BoolVar(&list, "list", false, "list preserved worktrees")
	fs.StringVar(&merge, "merge", "", "retry merge for slug")
	fs.StringVar(&clean, "clean", "", "delete worktree and branch for slug")
	fs.BoolVar(&cleanAll, "clean-all", false, "clean all preserved worktrees")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("recover: %w", err)
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	worktrees := listPreservedWorktrees(root)

	if merge != "" {
		return recoverMerge(root, merge, worktrees)
	}
	if clean != "" {
		return recoverClean(root, clean, worktrees)
	}
	if cleanAll {
		return recoverCleanAll(root, worktrees, format)
	}

	// Default: list (explicit or implicit)
	return recoverList(root, worktrees, format)
}

func recoverList(root string, worktrees []worktreeEntry, format string) error {
	if format == "json" {
		type wtJSON struct {
			Slug   string `json:"slug"`
			Path   string `json:"path"`
			Branch string `json:"branch"`
		}
		var items []wtJSON
		for _, wt := range worktrees {
			slug := filepath.Base(wt.Path)
			items = append(items, wtJSON{Slug: slug, Path: wt.Path, Branch: wt.Branch})
		}
		if items == nil {
			items = []wtJSON{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	if len(worktrees) == 0 {
		fmt.Println("No preserved worktrees found.")
		return nil
	}

	fmt.Printf("Preserved worktrees (%d):\n\n", len(worktrees))
	for _, wt := range worktrees {
		slug := filepath.Base(wt.Path)
		fmt.Printf("  %s\n", slug)
		fmt.Printf("    Path:   %s\n", wt.Path)
		fmt.Printf("    Branch: %s\n", wt.Branch)
		fmt.Println()
	}
	fmt.Println("Actions:")
	fmt.Println("  belmont recover --merge <slug>    Retry merge with improved logic")
	fmt.Println("  belmont recover --clean <slug>    Delete worktree and branch")
	fmt.Println("  belmont recover --clean-all       Clean all preserved worktrees")
	return nil
}

func findWorktree(worktrees []worktreeEntry, slug string) *worktreeEntry {
	for _, wt := range worktrees {
		if filepath.Base(wt.Path) == slug {
			return &wt
		}
	}
	return nil
}

func recoverMerge(root, slug string, worktrees []worktreeEntry) error {
	wt := findWorktree(worktrees, slug)
	if wt == nil {
		return fmt.Errorf("no preserved worktree found for slug: %s", slug)
	}

	commitMsg := fmt.Sprintf("belmont: merge recovered %s", slug)
	cfg := loopConfig{Root: root}

	if err := attemptMerge(cfg, commitMsg, wt.Branch, slug); err != nil {
		return fmt.Errorf("merge failed for %s: %w", slug, err)
	}

	// Clean up reconciliation report if it exists
	os.Remove(filepath.Join(root, ".belmont", "reconciliation-report.json"))

	// Clean up worktree and branch
	removeWorktree(root, wt.Path, slug)

	delCmd := exec.Command("git", "branch", "-d", wt.Branch)
	delCmd.Dir = root
	delCmd.Run()

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Recovered and merged %s\033[0m\n", slug)
	return nil
}

func recoverClean(root, slug string, worktrees []worktreeEntry) error {
	wt := findWorktree(worktrees, slug)
	if wt == nil {
		return fmt.Errorf("no preserved worktree found for slug: %s", slug)
	}

	removeWorktree(root, wt.Path, slug)

	delCmd := exec.Command("git", "branch", "-D", wt.Branch)
	delCmd.Dir = root
	delCmd.Run()

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Cleaned up %s\033[0m\n", slug)
	return nil
}

func recoverCleanAll(root string, worktrees []worktreeEntry, format string) error {
	if len(worktrees) == 0 {
		if format != "json" {
			fmt.Println("No preserved worktrees to clean.")
		}
		return nil
	}

	for _, wt := range worktrees {
		slug := filepath.Base(wt.Path)
		removeWorktree(root, wt.Path, slug)

		delCmd := exec.Command("git", "branch", "-D", wt.Branch)
		delCmd.Dir = root
		delCmd.Run()

		fmt.Fprintf(os.Stderr, "  \033[32m✓ Cleaned up %s\033[0m\n", slug)
	}
	return nil
}

