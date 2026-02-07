package grading

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type evaluatorFunc func(context.Context, Request, CheckSpec) (evaluation, error)

type DefaultGrader struct {
	registry map[string]evaluatorFunc
}

func NewGrader() *DefaultGrader {
	g := &DefaultGrader{registry: map[string]evaluatorFunc{}}
	g.registry["file_exists"] = g.evalFileExists
	g.registry["file_text_exact"] = g.evalFileTextExact
	g.registry["file_lines_count"] = g.evalFileLinesCount
	g.registry["file_lines_match_regex"] = g.evalFileLinesMatchRegex
	g.registry["file_sorted"] = g.evalFileSorted
	g.registry["command_output_equals_file"] = g.evalCommandOutputEqualsFile
	g.registry["cmdlog_contains_regex"] = g.evalCmdlogContainsRegex
	g.registry["cmdlog_forbids_regex"] = g.evalCmdlogForbidsRegex
	return g
}

func (g *DefaultGrader) Grade(ctx context.Context, req Request) (Result, error) {
	if req.FinishedAt.IsZero() {
		req.FinishedAt = time.Now()
	}
	if req.StartedAt.IsZero() {
		req.StartedAt = req.FinishedAt
	}

	result := Result{
		Kind:          ResultKind,
		SchemaVersion: SchemaVersion,
		AppVersion:    req.AppVersion,
		PackID:        req.PackID,
		PackVersion:   req.PackVersion,
		LevelID:       req.LevelID,
		Run: RunInfo{
			RunID:            req.RunID,
			Attempt:          max(1, req.Attempt),
			StartedAtUnixMS:  req.StartedAt.UnixMilli(),
			FinishedAtUnixMS: req.FinishedAt.UnixMilli(),
			DurationMS:       max64(0, req.FinishedAt.Sub(req.StartedAt).Milliseconds()),
		},
		EngineDebug: EngineDebug{
			Engine:        req.Engine,
			ContainerName: req.Container,
			ImageRef:      req.ImageRef,
		},
	}

	bonusPoints := 0
	requiredFailed := false
	patternCounts := []PatternCount{}

	for _, check := range req.Checks {
		eval, err := g.evaluateCheck(ctx, req, check)
		if err != nil {
			return Result{}, err
		}
		msg := eval.Message
		if !eval.Passed && check.OnFailMessage != "" {
			msg = check.OnFailMessage
		}
		if eval.Passed && check.OnPassMessage != "" {
			msg = check.OnPassMessage
		}
		cr := CheckResult{
			ID:            check.ID,
			Type:          check.Type,
			Required:      check.Required,
			Passed:        eval.Passed,
			PointsAwarded: eval.PointsAwarded,
			Summary:       eval.Summary,
			Message:       msg,
		}
		if eval.Artifact != nil {
			result.Artifacts = append(result.Artifacts, *eval.Artifact)
			cr.Artifacts = append(cr.Artifacts, ArtifactRef{Kind: eval.Artifact.Kind, Ref: eval.Artifact.Ref})
		}
		if eval.PatternCount != nil {
			patternCounts = append(patternCounts, *eval.PatternCount)
		}
		if !eval.Passed && check.Required {
			requiredFailed = true
		}
		if eval.Passed && !check.Required {
			bonusPoints += check.Points
		}
		result.Checks = append(result.Checks, cr)
	}

	result.Passed = !requiredFailed

	base := defaultInt(req.BasePoints, 1000)
	grace := defaultInt(req.TimeGraceSeconds, 60)
	timePenaltyPerSec := defaultInt(req.TimePenaltyPerSecond, 1)
	hintPenalty := defaultInt(req.HintPenaltyPoints, 80)
	resetPenalty := defaultInt(req.ResetPenaltyPoints, 120)

	durationSec := int(result.Run.DurationMS / 1000)
	timePenaltyPoints := 0
	if durationSec > grace {
		timePenaltyPoints = (durationSec - grace) * timePenaltyPerSec
	}
	hintPenaltyPoints := req.HintsUsed * hintPenalty
	resetPenaltyPoints := req.Resets * resetPenalty

	total := base - timePenaltyPoints - hintPenaltyPoints - resetPenaltyPoints + bonusPoints
	if total < 0 {
		total = 0
	}
	result.Score = Score{
		BasePoints:          base,
		TimeGraceSeconds:    grace,
		TimePenaltyPoints:   timePenaltyPoints,
		HintPenaltyPoints:   hintPenaltyPoints,
		ResetPenaltyPoints:  resetPenaltyPoints,
		OptionalBonusPoints: bonusPoints,
		TotalPoints:         total,
		Breakdown: []ScoreDelta{
			{Kind: "time", Points: -timePenaltyPoints, Description: "Time penalty after grace"},
			{Kind: "hint", Points: -hintPenaltyPoints, Description: "Hints revealed"},
			{Kind: "reset", Points: -resetPenaltyPoints, Description: "Resets used"},
			{Kind: "bonus", Points: bonusPoints, Description: "Optional checks / cmdlog bonuses"},
		},
	}
	if len(patternCounts) > 0 {
		result.CmdlogAnalysis = &CmdlogAnalysis{CmdCount: countCmdlogEntries(req.WorkDir), MatchedPatterns: patternCounts}
	}
	return result, nil
}

func (g *DefaultGrader) evaluateCheck(ctx context.Context, req Request, check CheckSpec) (evaluation, error) {
	evaluator, ok := g.registry[check.Type]
	if !ok {
		return evaluation{Passed: false, Summary: "unknown check", Message: "unknown check type: " + check.Type}, nil
	}
	return evaluator(ctx, req, check)
}

func (g *DefaultGrader) evalFileExists(_ context.Context, req Request, check CheckSpec) (evaluation, error) {
	path := resolveWorkPath(req.WorkDir, check.Path)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return evaluation{Passed: false, Summary: "file missing", Message: "file not found"}, nil
		}
		return evaluation{}, err
	}
	return evaluation{Passed: true, Summary: "file exists", Message: "ok"}, nil
}

func (g *DefaultGrader) evalFileTextExact(_ context.Context, req Request, check CheckSpec) (evaluation, error) {
	path := resolveWorkPath(req.WorkDir, check.Path)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return evaluation{Passed: false, Summary: "file missing", Message: "file not found"}, nil
		}
		return evaluation{}, err
	}
	expected := normalizeText(check.Expected, check.Normalize)
	actual := normalizeText(string(content), check.Normalize)
	if actual == expected {
		return evaluation{Passed: true, Summary: "content matches", Message: "ok"}, nil
	}
	artifact := Artifact{
		Ref:         "diff_" + safeID(check.ID),
		Kind:        "unified_diff",
		Title:       fmt.Sprintf("%s vs expected", check.Path),
		TextPreview: buildUnifiedDiff(expected, actual),
	}
	return evaluation{Passed: false, Summary: "content mismatch", Message: "file content differs", Artifact: &artifact}, nil
}

func (g *DefaultGrader) evalFileLinesCount(_ context.Context, req Request, check CheckSpec) (evaluation, error) {
	path := resolveWorkPath(req.WorkDir, check.Path)
	count, err := countFileLines(path)
	if err != nil {
		if os.IsNotExist(err) {
			return evaluation{Passed: false, Summary: "file missing", Message: "file not found"}, nil
		}
		return evaluation{}, err
	}
	if check.Equals > 0 {
		if count == check.Equals {
			return evaluation{Passed: true, Summary: "line count matches", Message: "ok"}, nil
		}
		return evaluation{Passed: false, Summary: "line count mismatch", Message: fmt.Sprintf("expected %d lines got %d", check.Equals, count)}, nil
	}
	if check.Min != nil && count < *check.Min {
		return evaluation{Passed: false, Summary: "line count below minimum", Message: fmt.Sprintf("min %d got %d", *check.Min, count)}, nil
	}
	if check.Max != nil && count > *check.Max {
		return evaluation{Passed: false, Summary: "line count above maximum", Message: fmt.Sprintf("max %d got %d", *check.Max, count)}, nil
	}
	return evaluation{Passed: true, Summary: "line count within range", Message: "ok"}, nil
}

func (g *DefaultGrader) evalFileLinesMatchRegex(_ context.Context, req Request, check CheckSpec) (evaluation, error) {
	path := resolveWorkPath(req.WorkDir, check.Path)
	lines, err := readLines(path)
	if err != nil {
		if os.IsNotExist(err) {
			return evaluation{Passed: false, Summary: "file missing", Message: "file not found"}, nil
		}
		return evaluation{}, err
	}
	r, err := regexp.Compile(check.Pattern)
	if err != nil {
		return evaluation{}, err
	}
	mode := check.Mode
	if mode == "" {
		mode = "all_lines"
	}
	matches := 0
	for _, line := range lines {
		if r.MatchString(line) {
			matches++
		}
	}
	switch mode {
	case "all_lines":
		if matches == len(lines) {
			return evaluation{Passed: true, Summary: "all lines match regex", Message: "ok"}, nil
		}
		return evaluation{Passed: false, Summary: "regex mismatch", Message: fmt.Sprintf("matched %d of %d", matches, len(lines))}, nil
	case "any_line":
		if matches > 0 {
			return evaluation{Passed: true, Summary: "at least one line matches", Message: "ok"}, nil
		}
		return evaluation{Passed: false, Summary: "no lines matched regex", Message: "0 matches"}, nil
	case "min_matches":
		if matches >= check.MinMatches {
			return evaluation{Passed: true, Summary: "minimum matches met", Message: "ok"}, nil
		}
		return evaluation{Passed: false, Summary: "minimum matches not met", Message: fmt.Sprintf("need %d got %d", check.MinMatches, matches)}, nil
	default:
		return evaluation{Passed: false, Summary: "invalid mode", Message: "unsupported regex mode"}, nil
	}
}

func (g *DefaultGrader) evalFileSorted(_ context.Context, req Request, check CheckSpec) (evaluation, error) {
	path := resolveWorkPath(req.WorkDir, check.Path)
	lines, err := readLines(path)
	if err != nil {
		if os.IsNotExist(err) {
			return evaluation{Passed: false, Summary: "file missing", Message: "file not found"}, nil
		}
		return evaluation{}, err
	}
	if len(lines) <= 1 {
		return evaluation{Passed: true, Summary: "sorted", Message: "ok"}, nil
	}
	column := check.Column
	if column <= 0 {
		column = 1
	}
	for i := 1; i < len(lines); i++ {
		prev := extractSortKey(lines[i-1], check, column)
		curr := extractSortKey(lines[i], check, column)
		cmp := compareSort(prev, curr, check.Key, check.IgnoreCase)
		if check.Order == "desc" {
			if cmp < 0 {
				return evaluation{Passed: false, Summary: "not sorted descending", Message: fmt.Sprintf("line %d out of order", i+1)}, nil
			}
		} else {
			if cmp > 0 {
				return evaluation{Passed: false, Summary: "not sorted ascending", Message: fmt.Sprintf("line %d out of order", i+1)}, nil
			}
		}
		if check.Unique && cmp == 0 {
			return evaluation{Passed: false, Summary: "not unique", Message: fmt.Sprintf("duplicate sort key near line %d", i+1)}, nil
		}
	}
	return evaluation{Passed: true, Summary: "sorted", Message: "ok"}, nil
}

func (g *DefaultGrader) evalCommandOutputEqualsFile(ctx context.Context, req Request, check CheckSpec) (evaluation, error) {
	out, err := runCommand(ctx, req, check.Command, check.TimeoutSeconds)
	if err != nil {
		return evaluation{}, err
	}
	filePath := resolveWorkPath(req.WorkDir, check.CompareToPath)
	b, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return evaluation{Passed: false, Summary: "file missing", Message: "compare file not found"}, nil
		}
		return evaluation{}, err
	}
	expected := normalizeText(string(out), check.Normalize)
	actual := normalizeText(string(b), check.Normalize)
	if actual == expected {
		return evaluation{Passed: true, Summary: "command output matches file", Message: "ok"}, nil
	}
	artifact := Artifact{
		Ref:         "diff_" + safeID(check.ID),
		Kind:        "unified_diff",
		Title:       fmt.Sprintf("%s output vs %s", check.Command, check.CompareToPath),
		TextPreview: buildUnifiedDiff(expected, actual),
	}
	return evaluation{Passed: false, Summary: "command output mismatch", Message: "output differs", Artifact: &artifact}, nil
}

func (g *DefaultGrader) evalCmdlogContainsRegex(_ context.Context, req Request, check CheckSpec) (evaluation, error) {
	body, err := os.ReadFile(filepath.Join(req.WorkDir, ".dojo_cmdlog"))
	if err != nil {
		if os.IsNotExist(err) {
			return evaluation{Passed: false, Summary: "cmdlog missing", Message: "no .dojo_cmdlog file found"}, nil
		}
		return evaluation{}, err
	}
	r, err := regexp.Compile(check.Pattern)
	if err != nil {
		return evaluation{}, err
	}
	matches := r.FindAllStringIndex(string(body), -1)
	min := check.MinCount
	if min <= 0 {
		min = 1
	}
	if len(matches) >= min {
		return evaluation{Passed: true, Summary: "pattern found", Message: "ok", PatternCount: &PatternCount{PatternID: check.ID, Count: len(matches)}}, nil
	}
	return evaluation{Passed: false, Summary: "pattern not found", Message: fmt.Sprintf("need %d matches got %d", min, len(matches))}, nil
}

func (g *DefaultGrader) evalCmdlogForbidsRegex(_ context.Context, req Request, check CheckSpec) (evaluation, error) {
	body, err := os.ReadFile(filepath.Join(req.WorkDir, ".dojo_cmdlog"))
	if err != nil {
		if os.IsNotExist(err) {
			return evaluation{Passed: true, Summary: "cmdlog missing", Message: "ok"}, nil
		}
		return evaluation{}, err
	}
	r, err := regexp.Compile(check.Pattern)
	if err != nil {
		return evaluation{}, err
	}
	if r.Match(body) {
		return evaluation{Passed: false, Summary: "forbidden pattern found", Message: "cmdlog contains forbidden pattern"}, nil
	}
	return evaluation{Passed: true, Summary: "forbidden pattern avoided", Message: "ok"}, nil
}

func resolveWorkPath(workDir, p string) string {
	if p == "" {
		return workDir
	}
	if strings.HasPrefix(p, "/work/") {
		return filepath.Join(workDir, strings.TrimPrefix(p, "/work/"))
	}
	if p == "/work" {
		return workDir
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(workDir, p)
}

func runCommand(ctx context.Context, req Request, command string, timeoutSeconds int) ([]byte, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 3
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if req.Engine == "docker" || req.Engine == "podman" {
		args := []string{"exec", "-i", "-w", "/work", req.Container, "bash", "-lc", command}
		out, err := exec.CommandContext(cctx, req.Engine, args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("%s exec failed: %s", req.Engine, strings.TrimSpace(string(out)))
		}
		return out, nil
	}
	cmd := exec.CommandContext(cctx, "bash", "-lc", command)
	cmd.Dir = req.WorkDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("command failed: %s", strings.TrimSpace(string(out)))
	}
	return out, nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	out := []string{}
	for s.Scan() {
		out = append(out, s.Text())
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func countFileLines(path string) (int, error) {
	lines, err := readLines(path)
	if err != nil {
		return 0, err
	}
	return len(lines), nil
}

func extractSortKey(line string, check CheckSpec, column int) string {
	fields := []string{}
	if check.Split.Kind == "delimiter" {
		fields = strings.Split(line, check.Split.Delimiter)
	} else {
		fields = strings.Fields(line)
	}
	idx := column - 1
	if idx < 0 || idx >= len(fields) {
		return ""
	}
	return fields[idx]
}

func compareSort(a, b, key string, ignoreCase bool) int {
	if ignoreCase {
		a = strings.ToLower(a)
		b = strings.ToLower(b)
	}
	switch key {
	case "numeric":
		fa, _ := strconv.ParseFloat(a, 64)
		fb, _ := strconv.ParseFloat(b, 64)
		if fa < fb {
			return -1
		}
		if fa > fb {
			return 1
		}
		return 0
	case "human":
		return strings.Compare(a, b)
	default:
		return strings.Compare(a, b)
	}
}

func normalizeText(s string, n NormalizeSpec) string {
	if n.Newlines == "" {
		n.Newlines = "any"
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if n.Newlines == "crlf" {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	if n.TrimTrailingWhitespace {
		lines := strings.Split(s, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " \t")
		}
		s = strings.Join(lines, "\n")
	}
	if n.TrimFinalNewline {
		s = strings.TrimSuffix(s, "\n")
		s = strings.TrimSuffix(s, "\r")
	}
	return s
}

func buildUnifiedDiff(expected, actual string) string {
	exp := strings.Split(strings.TrimSuffix(expected, "\n"), "\n")
	act := strings.Split(strings.TrimSuffix(actual, "\n"), "\n")
	maxLen := len(exp)
	if len(act) > maxLen {
		maxLen = len(act)
	}
	var b strings.Builder
	b.WriteString("--- expected\n+++ actual\n")
	for i := 0; i < maxLen; i++ {
		var e, a string
		if i < len(exp) {
			e = exp[i]
		}
		if i < len(act) {
			a = act[i]
		}
		if e == a {
			continue
		}
		if e != "" {
			b.WriteString("-" + e + "\n")
		}
		if a != "" {
			b.WriteString("+" + a + "\n")
		}
	}
	return b.String()
}

func safeID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "artifact"
	}
	s = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`).ReplaceAllString(s, "_")
	return s
}

func countCmdlogEntries(workDir string) int {
	body, err := os.ReadFile(filepath.Join(workDir, ".dojo_cmdlog"))
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return 0
	}
	return len(lines)
}

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
