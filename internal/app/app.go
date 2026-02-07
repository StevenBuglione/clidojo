package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"clidojo/internal/devtools"
	"clidojo/internal/grading"
	"clidojo/internal/levels"
	"clidojo/internal/sandbox"
	"clidojo/internal/state"
	"clidojo/internal/telemetry"
	"clidojo/internal/term"
	"clidojo/internal/ui"

	"github.com/google/uuid"
)

type App struct {
	cfg Config

	logger  *telemetry.JSONLogger
	store   *state.SQLiteStore
	loader  *levels.FSLoader
	grader  *grading.DefaultGrader
	sandbox *sandbox.Manager
	demo    *devtools.Manager

	view *ui.Root
	term *term.TerminalPane

	sessionID string
	engine    sandbox.EngineInfo

	packs []levels.Pack
	pack  levels.Pack
	level levels.Level

	handle sandbox.Handle
	runID  int64

	startTime    time.Time
	hintsUsed    int
	hintRevealed int
	resetCount   int
	checkFails   int
	checkAttempt int
	menuOpen     bool
	hintsOpen    bool
	goalOpen     bool
	journalOpen  bool

	checkStatus map[string]string
	lastResult  grading.Result

	devMu     sync.Mutex
	devServer *http.Server
	devState  struct {
		State     string
		Demo      string
		RenderSeq int
	}
}

func New(cfg Config) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}

	logger, err := telemetry.NewJSONLogger(cfg.LogPath)
	if err != nil {
		return nil, err
	}

	store, err := state.NewSQLite(filepath.Join(cfg.DataDir, "state.db"))
	if err != nil {
		_ = logger.Close()
		return nil, err
	}
	if err := store.EnsureSchema(context.Background()); err != nil {
		_ = store.Close()
		_ = logger.Close()
		return nil, err
	}

	loader := levels.NewLoader()
	packs, err := loader.LoadPacks(context.Background(), "packs")
	if err != nil {
		_ = store.Close()
		_ = logger.Close()
		return nil, err
	}
	if len(packs) == 0 || len(packs[0].LoadedLevels) == 0 {
		return nil, fmt.Errorf("no packs/levels available under packs/")
	}

	termPane := term.NewTerminalPane(func() {})
	view := ui.New(ui.Options{ASCIIOnly: cfg.ASCIIOnly, Debug: cfg.DebugLayout, TermPane: termPane})

	a := &App{
		cfg:         cfg,
		logger:      logger,
		store:       store,
		loader:      loader,
		grader:      grading.NewGrader(),
		sandbox:     sandbox.NewManager(cfg.SandboxMode),
		demo:        devtools.NewManager(),
		view:        view,
		term:        termPane,
		sessionID:   uuid.NewString(),
		packs:       packs,
		pack:        packs[0],
		level:       packs[0].LoadedLevels[0],
		checkStatus: map[string]string{},
	}
	view.SetController(a)
	return a, nil
}

func (a *App) Run(ctx context.Context) error {
	a.logger.Info("app.start", map[string]any{"session": a.sessionID, "sandbox": a.cfg.SandboxMode})

	engine, err := a.sandbox.Detect(ctx, a.cfg.EngineOverride)
	if err != nil {
		a.logger.Error("engine.detect_failed", map[string]any{"error": err.Error()})
		a.engine = sandbox.EngineInfo{Name: "unavailable"}
		a.view.SetSetupError("No supported container engine found", err.Error())
		if a.cfg.SandboxMode != "mock" {
			a.view.SetPlayingState(ui.PlayingState{ModeLabel: "Setup Wizard", PackID: a.pack.PackID, LevelID: a.level.LevelID, Engine: "missing"})
			return a.view.Run()
		}
	} else {
		a.engine = engine
		a.logger.Info("engine.detected", map[string]any{"engine": engine.Name, "version": engine.Version})
		_ = a.sandbox.CleanupOrphans(ctx, a.sessionID)
	}

	a.logger.Info("level.starting", map[string]any{"pack": a.pack.PackID, "level": a.level.LevelID})
	if err := a.startLevel(ctx, true); err != nil {
		a.logger.Error("level.start_failed", map[string]any{"error": err.Error()})
		return err
	}
	a.logger.Info("level.started", map[string]any{"pack": a.pack.PackID, "level": a.level.LevelID})

	if a.cfg.Dev {
		if err := a.startDevHTTP(); err != nil {
			return err
		}
		if a.cfg.DemoScenario != "" {
			a.applyDemoScenario(context.Background(), a.cfg.DemoScenario)
		}
	}

	return a.view.Run()
}

func (a *App) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if a.devServer != nil {
		_ = a.devServer.Shutdown(ctx)
	}
	if a.handle != nil {
		_ = a.handle.Stop(ctx)
	}
	_ = a.term.Stop()
	_ = a.store.Close()
	_ = a.logger.Close()
}

func (a *App) startLevel(ctx context.Context, newRun bool) error {
	if a.handle != nil {
		_ = a.handle.Stop(ctx)
		a.handle = nil
	}
	_ = a.term.Stop()
	a.view.SetResult(ui.ResultState{})
	a.view.SetReferenceText("", false)
	a.view.SetDiffText("", false)

	workDir := filepath.Join(a.cfg.DataDir, "work", a.sessionID, a.level.LevelID)
	a.logger.Info("level.stage_workdir", map[string]any{"workdir": workDir})
	if err := a.loader.StageWorkdir(a.level, workDir); err != nil {
		return err
	}

	image := a.pack.Image.Ref
	if a.level.Image.Ref != "" {
		image = a.level.Image.Ref
	}
	readOnly := true
	if a.level.Sandbox.ReadOnlyRoot != nil {
		readOnly = *a.level.Sandbox.ReadOnlyRoot
	}
	tmpfs := make([]sandbox.TmpfsMount, 0, len(a.level.Sandbox.Tmpfs))
	for _, tm := range a.level.Sandbox.Tmpfs {
		tmpfs = append(tmpfs, sandbox.TmpfsMount{Mount: tm.Mount, Options: tm.Options})
	}

	handle, err := a.sandbox.StartLevel(ctx, sandbox.StartSpec{
		SessionID:     a.sessionID,
		PackID:        a.pack.PackID,
		LevelID:       a.level.LevelID,
		ContainerName: containerName(a.sessionID, a.level.LevelID),
		Image:         image,
		DatasetDir:    a.level.DatasetHostPath,
		DatasetMount:  a.level.Filesystem.Dataset.MountPoint,
		WorkDir:       workDir,
		WorkMount:     a.level.Filesystem.Work.MountPoint,
		ShellProgram:  a.level.Shell.Program,
		ShellArgs:     a.level.Shell.Args,
		ShellCWD:      a.level.Shell.CWD,
		ShellEnv:      a.level.Shell.Env,
		Network:       a.level.Sandbox.Network,
		ReadOnlyRoot:  readOnly,
		CPU:           a.level.Sandbox.CPU,
		MemoryMB:      a.level.Sandbox.MemoryMB,
		PidsLimit:     a.level.Sandbox.PidsLimit,
		Tmpfs:         tmpfs,
	})
	if err != nil {
		return err
	}
	a.logger.Info("sandbox.started", map[string]any{"container": handle.ContainerName(), "mock": handle.IsMock()})
	a.handle = handle
	if current := a.sandbox.CurrentEngine(); current != "" {
		a.engine.Name = current
	}

	if newRun {
		runID, err := a.store.StartLevelRun(ctx, state.LevelRun{
			SessionID: a.sessionID,
			PackID:    a.pack.PackID,
			LevelID:   a.level.LevelID,
			StartTS:   time.Now().UTC(),
		})
		if err != nil {
			return err
		}
		a.runID = runID
		a.startTime = time.Now()
		a.hintsUsed = 0
		a.hintRevealed = 0
		a.resetCount = 0
		a.checkFails = 0
		a.checkAttempt = 0
	}
	a.lastResult = grading.Result{}
	a.checkStatus = map[string]string{}
	for _, c := range a.level.Checks {
		a.checkStatus[c.ID] = "pending"
	}

	if handle.IsMock() {
		a.logger.Info("term.mode", map[string]any{"mode": "playback"})
		if err := os.WriteFile(filepath.Join(workDir, ".dojo_cmdlog"), []byte(a.demo.MockCmdLog(a.level.LevelID)), 0o644); err != nil {
			return err
		}
		if err := a.term.StartPlayback(ctx, a.demo.PlaybackFrames(a.level.LevelID, "playing"), false); err != nil {
			return err
		}
	} else {
		a.logger.Info("term.mode", map[string]any{"mode": "pty"})
		if err := a.term.Start(ctx, handle.ShellCommand(), handle.Cwd(), handle.Env()); err != nil {
			return err
		}
	}

	a.syncPlayingState(a.level.Scoring.BasePoints, a.badgesFor(false))
	a.view.FlashStatus("Level ready")
	a.setDevState("playing", "playing")
	if err := a.demo.SetState(ctx, "", "playing", true); err != nil {
		a.logger.Error("dev_state.write_failed", map[string]any{"state": "playing", "error": err.Error()})
	}
	return nil
}

func (a *App) syncPlayingState(score int, badges []string) {
	if badges == nil {
		badges = a.badgesFor(a.lastResult.Passed)
	}
	checks := make([]ui.CheckRow, 0, len(a.level.Checks))
	for _, c := range a.level.Checks {
		checks = append(checks, ui.CheckRow{ID: c.ID, Description: c.Description, Status: a.checkStatus[c.ID]})
	}
	a.view.SetPlayingState(ui.PlayingState{
		ModeLabel: a.modeLabel(),
		PackID:    a.pack.PackID,
		LevelID:   a.level.LevelID,
		Objective: a.level.Objective.Bullets,
		Checks:    checks,
		Hints:     a.buildHintRows(),
		Engine:    a.engine.Name,
		StartedAt: a.startTime,
		HintsUsed: a.hintsUsed,
		Resets:    a.resetCount,
		Score:     score,
		Streak:    0,
		Badges:    badges,
	})
}

func (a *App) buildHintRows() []ui.HintRow {
	if len(a.level.Hints) == 0 {
		return []ui.HintRow{{Text: "Use F5 to run checks.", Revealed: true}}
	}
	rows := make([]ui.HintRow, 0, len(a.level.Hints))
	for i, h := range a.level.Hints {
		revealed := i < a.hintRevealed
		unlocked, reason := a.hintUnlocked(i)
		rows = append(rows, ui.HintRow{
			Text:       h.TextMD,
			Revealed:   revealed,
			Locked:     !unlocked && !revealed,
			LockReason: reason,
		})
	}
	return rows
}

func (a *App) hintUnlocked(idx int) (bool, string) {
	if idx <= 0 {
		return true, ""
	}
	if idx >= len(a.level.Hints) {
		return false, ""
	}
	h := a.level.Hints[idx]
	elapsed := int(time.Since(a.startTime).Seconds())
	if h.Unlock.AfterSeconds > 0 && elapsed >= h.Unlock.AfterSeconds {
		return true, ""
	}
	if h.Unlock.AfterFailedChecks > 0 && a.checkFails >= h.Unlock.AfterFailedChecks {
		return true, ""
	}
	if h.Unlock.AfterReveals > 0 && a.hintRevealed >= h.Unlock.AfterReveals {
		return true, ""
	}
	parts := make([]string, 0, 3)
	if h.Unlock.AfterSeconds > 0 {
		parts = append(parts, fmt.Sprintf("after %ds", h.Unlock.AfterSeconds))
	}
	if h.Unlock.AfterFailedChecks > 0 {
		parts = append(parts, fmt.Sprintf("after %d failed checks", h.Unlock.AfterFailedChecks))
	}
	if h.Unlock.AfterReveals > 0 {
		parts = append(parts, fmt.Sprintf("after %d reveals", h.Unlock.AfterReveals))
	}
	if len(parts) == 0 {
		return true, ""
	}
	return false, strings.Join(parts, " or ")
}

func (a *App) modeLabel() string {
	if a.cfg.Dev {
		return "Daily Drill"
	}
	return "Free Play"
}

func (a *App) OnCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	a.view.FlashStatus("Checking...")
	a.checkAttempt++

	checks := a.levelChecksForGrading()
	for _, bonus := range a.level.Scoring.CmdlogBonuses {
		checks = append(checks, grading.CheckSpec{
			ID:       bonus.ID,
			Type:     "cmdlog_contains_regex",
			Required: false,
			Points:   bonus.Points,
			Pattern:  bonus.Pattern,
			MinCount: 1,
		})
	}

	started := time.Now()
	var (
		result grading.Result
		err    error
	)
	if a.handle.IsMock() {
		result = a.demo.MockGrade(devtools.MockGradeRequest{
			LevelID:        a.level.LevelID,
			Checks:         checks,
			Attempt:        a.checkAttempt,
			BasePoints:     a.level.Scoring.BasePoints,
			HintsUsed:      a.hintsUsed,
			Resets:         a.resetCount,
			TimePenalty:    a.level.Scoring.TimePenaltyPerSecond,
			HintPenalty:    a.level.Scoring.HintPenaltyPoints,
			ResetPenalty:   a.level.Scoring.ResetPenaltyPoints,
			GraceSeconds:   a.level.Scoring.TimeGraceSeconds,
			ElapsedSeconds: int(time.Since(a.startTime).Seconds()),
			PackID:         a.pack.PackID,
			PackVersion:    a.pack.Version,
		})
	} else {
		result, err = a.grader.Grade(ctx, grading.Request{
			AppVersion:           "0.1.0",
			PackID:               a.pack.PackID,
			PackVersion:          a.pack.Version,
			LevelID:              a.level.LevelID,
			RunID:                fmt.Sprintf("%s-%d", a.sessionID, a.runID),
			Attempt:              a.checkAttempt,
			StartedAt:            started,
			FinishedAt:           time.Now(),
			Engine:               a.engine.Name,
			Container:            a.handle.ContainerName(),
			ImageRef:             ifThenElse(a.level.Image.Ref != "", a.level.Image.Ref, a.pack.Image.Ref),
			WorkDir:              a.handle.WorkDir(),
			Checks:               checks,
			BasePoints:           a.level.Scoring.BasePoints,
			TimeGraceSeconds:     a.level.Scoring.TimeGraceSeconds,
			TimePenaltyPerSecond: a.level.Scoring.TimePenaltyPerSecond,
			HintPenaltyPoints:    a.level.Scoring.HintPenaltyPoints,
			ResetPenaltyPoints:   a.level.Scoring.ResetPenaltyPoints,
			HintsUsed:            a.hintsUsed,
			Resets:               a.resetCount,
		})
	}
	if err != nil {
		a.view.FlashStatus("Check failed: " + err.Error())
		return
	}
	if result.Kind == "" {
		result.Kind = grading.ResultKind
		result.SchemaVersion = grading.SchemaVersion
		result.PackID = a.pack.PackID
		result.PackVersion = a.pack.Version
		result.LevelID = a.level.LevelID
	}
	if result.Run.Attempt == 0 {
		result.Run = grading.RunInfo{
			RunID:            fmt.Sprintf("%s-%d", a.sessionID, a.runID),
			Attempt:          a.checkAttempt,
			StartedAtUnixMS:  started.UnixMilli(),
			FinishedAtUnixMS: time.Now().UnixMilli(),
			DurationMS:       time.Since(started).Milliseconds(),
		}
	}
	if result.EngineDebug.Engine == "" {
		result.EngineDebug = grading.EngineDebug{Engine: a.engine.Name, ContainerName: a.handle.ContainerName(), ImageRef: ifThenElse(a.level.Image.Ref != "", a.level.Image.Ref, a.pack.Image.Ref)}
	}

	a.lastResult = result
	_ = a.store.RecordCheckAttempt(ctx, a.runID, result.Passed)

	rows := make([]ui.CheckResultRow, 0, len(result.Checks))
	for _, c := range result.Checks {
		rows = append(rows, ui.CheckResultRow{ID: c.ID, Passed: c.Passed, Message: firstNonEmpty(c.Message, c.Summary)})
		if _, ok := a.checkStatus[c.ID]; ok {
			status := "fail"
			if c.Passed {
				status = "pass"
			} else {
				a.checkFails++
			}
			a.checkStatus[c.ID] = status
		}
	}

	breakdown := make([]ui.BreakdownRow, 0, len(result.Score.Breakdown)+1)
	for _, row := range result.Score.Breakdown {
		breakdown = append(breakdown, ui.BreakdownRow{Label: row.Kind, Value: fmt.Sprintf("%d", row.Points)})
	}
	breakdown = append(breakdown, ui.BreakdownRow{Label: "total", Value: fmt.Sprintf("%d", result.Score.TotalPoints)})

	a.syncPlayingState(result.Score.TotalPoints, a.badgesFor(result.Passed))
	a.view.SetResult(ui.ResultState{
		Visible:          true,
		Passed:           result.Passed,
		Summary:          resultSummary(result.Passed),
		Checks:           rows,
		Score:            result.Score.TotalPoints,
		Breakdown:        breakdown,
		CanShowReference: result.Passed || a.level.Difficulty <= 2,
		CanOpenDiff:      len(result.Artifacts) > 0,
		PrimaryAction:    ifThenElse(result.Passed, "Continue", "Try again"),
	})

	if result.Passed {
		a.view.FlashStatus("PASS")
		a.setDevState("results_pass", "results_pass")
	} else {
		a.view.FlashStatus("FAIL")
		a.setDevState("results_fail", "results_fail")
	}
	if err := a.demo.SetState(context.Background(), "", a.devState.State, true); err != nil {
		a.logger.Error("dev_state.write_failed", map[string]any{"state": a.devState.State, "error": err.Error()})
	}
}

func (a *App) levelChecksForGrading() []grading.CheckSpec {
	out := make([]grading.CheckSpec, 0, len(a.level.Checks))
	for _, c := range a.level.Checks {
		required := c.Required == nil || *c.Required
		out = append(out, grading.CheckSpec{
			ID:             c.ID,
			Type:           c.Type,
			Description:    c.Description,
			Required:       required,
			Points:         c.Points,
			OnFailMessage:  c.OnFailMessage,
			OnPassMessage:  c.OnPassMessage,
			Path:           c.Path,
			Expected:       c.Expected,
			Normalize:      grading.NormalizeSpec(c.Normalize),
			Equals:         c.Equals,
			Min:            c.Min,
			Max:            c.Max,
			Pattern:        c.Pattern,
			Mode:           c.Mode,
			MinMatches:     c.MinMatches,
			Order:          c.Order,
			Key:            c.Key,
			Unique:         c.Unique,
			IgnoreCase:     c.IgnoreCase,
			Split:          grading.FileSplitSpec(c.Split),
			Column:         c.Column,
			Command:        c.Command,
			CompareToPath:  c.CompareToPath,
			TimeoutSeconds: c.TimeoutSeconds,
			MinCount:       c.MinCount,
		})
	}
	return out
}

func (a *App) OnReset() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a.resetCount++
	_ = a.store.IncrementReset(ctx, a.runID)
	if err := a.startLevel(ctx, false); err != nil {
		a.view.FlashStatus("reset failed: " + err.Error())
		return
	}
	a.view.FlashStatus("Level reset")
}

func (a *App) OnMenu() {
	a.menuOpen = !a.menuOpen
	a.view.SetMenuOpen(a.menuOpen)
	if a.menuOpen {
		a.setDevState("menu", "menu")
		if err := a.demo.SetState(context.Background(), "", "menu", true); err != nil {
			a.logger.Error("dev_state.write_failed", map[string]any{"state": "menu", "error": err.Error()})
		}
	}
}

func (a *App) OnHints() {
	a.hintsOpen = !a.hintsOpen
	a.view.SetHintsOpen(a.hintsOpen)
	if a.hintsOpen {
		a.syncPlayingState(currentScore(a), nil)
		a.setDevState("hints_open", "hints_open")
		if err := a.demo.SetState(context.Background(), "", "hints_open", true); err != nil {
			a.logger.Error("dev_state.write_failed", map[string]any{"state": "hints_open", "error": err.Error()})
		}
	}
}

func (a *App) OnRevealHint() {
	for idx := a.hintRevealed; idx < len(a.level.Hints); idx++ {
		if unlocked, reason := a.hintUnlocked(idx); unlocked {
			a.hintRevealed = idx + 1
			a.hintsUsed++
			a.syncPlayingState(currentScore(a), nil)
			a.view.FlashStatus(fmt.Sprintf("Revealed hint %d", idx+1))
			return
		} else {
			a.view.FlashStatus("Hint locked: " + reason)
			return
		}
	}
	a.view.FlashStatus("All hints already revealed")
}

func (a *App) OnGoal() {
	a.goalOpen = !a.goalOpen
	a.view.SetGoalOpen(a.goalOpen)
}

func (a *App) OnJournal() {
	a.journalOpen = !a.journalOpen
	if a.journalOpen {
		a.view.SetJournalEntries(a.readJournalEntries())
		a.setDevState("journal_open", "journal_open")
		if err := a.demo.SetState(context.Background(), "", "journal_open", true); err != nil {
			a.logger.Error("dev_state.write_failed", map[string]any{"state": "journal_open", "error": err.Error()})
		}
	}
	a.view.SetJournalOpen(a.journalOpen)
}

func (a *App) OnJournalExplainAI() {
	a.view.SetInfo("AI Explain", "AI explain is optional and currently disabled in this local build.", true)
}

func (a *App) OnChangeLevel() {
	a.advanceLevel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.startLevel(ctx, true); err != nil {
		a.view.FlashStatus("change level failed: " + err.Error())
		return
	}
	a.view.FlashStatus("Changed level to " + a.level.LevelID)
}

func (a *App) OnOpenSettings() {
	text := fmt.Sprintf("Sandbox: %s\nEngine: %s\nData dir: %s\nASCII only: %t\nDebug layout: %t\nKeep artifacts: %t\nDev HTTP: %s",
		a.cfg.SandboxMode, a.engine.Name, a.cfg.DataDir, a.cfg.ASCIIOnly, a.cfg.DebugLayout, a.cfg.KeepArtifacts, a.cfg.DevHTTP)
	a.view.SetInfo("Settings", text, true)
}

func (a *App) OnOpenStats() {
	summary, err := a.store.GetSummary(context.Background())
	if err != nil {
		a.view.SetInfo("Stats", "Failed to load stats: "+err.Error(), true)
		return
	}
	text := fmt.Sprintf("Level runs: %d\nCheck attempts: %d\nPasses: %d\nResets: %d", summary.LevelRuns, summary.Attempts, summary.Passes, summary.Resets)
	a.view.SetInfo("Stats", text, true)
}

func (a *App) OnNextLevel() {
	if !a.lastResult.Passed {
		return
	}
	a.advanceLevel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.startLevel(ctx, true); err != nil {
		a.view.FlashStatus("next level failed: " + err.Error())
		return
	}
	a.view.FlashStatus("Loaded next level: " + a.level.LevelID)
}

func (a *App) OnTryAgain() {
	a.view.SetResult(ui.ResultState{})
}

func (a *App) OnShowReferenceSolutions() {
	if !a.lastResult.Passed && a.level.Difficulty > 2 {
		a.view.FlashStatus("Reference solutions locked until pass")
		return
	}
	if len(a.level.ReferenceSolutions) == 0 {
		a.view.SetInfo("Reference solutions", "No reference solutions available.", true)
		return
	}
	var b strings.Builder
	for _, rs := range a.level.ReferenceSolutions {
		b.WriteString("### " + rs.Title + "\n")
		b.WriteString(rs.ScriptSH + "\n")
		if rs.ExplanationMD != "" {
			b.WriteString(rs.ExplanationMD + "\n")
		}
		b.WriteString("\n")
	}
	a.view.SetReferenceText(b.String(), true)
}

func (a *App) OnOpenDiff() {
	if len(a.lastResult.Artifacts) == 0 {
		a.view.FlashStatus("No diff artifacts available")
		return
	}
	var b strings.Builder
	for _, art := range a.lastResult.Artifacts {
		b.WriteString("## " + art.Title + "\n")
		b.WriteString(art.TextPreview)
		if !strings.HasSuffix(art.TextPreview, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	a.view.SetDiffText(b.String(), true)
}

func (a *App) OnQuit() {
	a.view.Stop()
}

func (a *App) OnResize(cols, rows int) {
	mode := ui.DetermineLayoutMode(cols, rows)
	if mode == ui.LayoutTooSmall {
		a.view.SetTooSmall(cols, rows)
		return
	}
	h := rows - 4
	if h < 1 {
		h = 1
	}
	w := cols - 2
	if mode == ui.LayoutWide {
		hudWidth := a.pack.Defaults.UI.HUDWidth
		if hudWidth <= 0 {
			hudWidth = 42
		}
		w = cols - hudWidth
	}
	if w < 10 {
		w = 10
	}
	_ = a.term.Resize(w, h)
}

func (a *App) OnTerminalInput(data []byte) {
	if len(data) == 0 {
		return
	}
	_ = a.term.SendInput(data)
}

func (a *App) applyDemoScenario(ctx context.Context, scenario string) {
	s := a.demo.Resolve(scenario)
	a.menuOpen = s.MenuOpen
	a.hintsOpen = s.HintsOpen
	a.goalOpen = s.GoalOpen
	a.journalOpen = s.JournalOpen
	if a.hintsOpen {
		a.hintRevealed = min(1, len(a.level.Hints))
	}

	if a.handle != nil && a.handle.IsMock() {
		_ = a.term.StartPlayback(ctx, a.demo.PlaybackFrames(a.level.LevelID, scenario), false)
	}

	a.syncPlayingState(currentScore(a), nil)
	a.view.SetMenuOpen(a.menuOpen)
	a.view.SetHintsOpen(a.hintsOpen)
	a.view.SetGoalOpen(a.goalOpen)
	if a.journalOpen {
		a.view.SetJournalEntries(a.readJournalEntries())
	}
	a.view.SetJournalOpen(a.journalOpen)

	if s.ResultPass != nil {
		passed := *s.ResultPass
		a.lastResult = grading.Result{
			Kind:          grading.ResultKind,
			SchemaVersion: grading.SchemaVersion,
			PackID:        a.pack.PackID,
			PackVersion:   a.pack.Version,
			LevelID:       a.level.LevelID,
			Passed:        passed,
			Run:           grading.RunInfo{RunID: "demo", Attempt: 1, StartedAtUnixMS: time.Now().Add(-2 * time.Second).UnixMilli(), FinishedAtUnixMS: time.Now().UnixMilli(), DurationMS: 2000},
			Score:         grading.Score{BasePoints: 1000, TimePenaltyPoints: 20, HintPenaltyPoints: 80, ResetPenaltyPoints: 0, OptionalBonusPoints: 0, TotalPoints: 900, Breakdown: []grading.ScoreDelta{{Kind: "time", Points: -20, Description: "Time penalty after grace"}, {Kind: "hint", Points: -80, Description: "Hint revealed"}}},
			Checks:        []grading.CheckResult{{ID: "demo", Type: "file_exists", Required: true, Passed: passed, Summary: "deterministic", Message: "deterministic"}},
			EngineDebug:   grading.EngineDebug{Engine: a.engine.Name, ContainerName: a.handle.ContainerName(), ImageRef: ifThenElse(a.level.Image.Ref != "", a.level.Image.Ref, a.pack.Image.Ref)},
		}
		if !passed {
			a.lastResult.Artifacts = []grading.Artifact{{Ref: "diff_demo", Kind: "unified_diff", Title: "demo diff", TextPreview: "--- expected\n+++ actual\n-good\n+bad\n"}}
		}
		a.view.SetResult(ui.ResultState{
			Visible:          true,
			Passed:           passed,
			Summary:          "Demo scenario",
			Checks:           []ui.CheckResultRow{{ID: "demo", Passed: passed, Message: "deterministic"}},
			Score:            900,
			Breakdown:        []ui.BreakdownRow{{Label: "time", Value: "-20"}, {Label: "hint", Value: "-80"}, {Label: "total", Value: "900"}},
			CanShowReference: passed || a.level.Difficulty <= 2,
			CanOpenDiff:      !passed,
			PrimaryAction:    ifThenElse(passed, "Continue", "Try again"),
		})
	}

	a.setDevState(s.Name, s.Name)
	if err := a.demo.SetState(ctx, "", s.Name, true); err != nil {
		a.logger.Error("dev_state.write_failed", map[string]any{"state": s.Name, "error": err.Error()})
	}
}

func (a *App) readJournalEntries() []ui.JournalEntry {
	path := filepath.Join(a.handle.WorkDir(), ".dojo_cmdlog")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	entries := make([]ui.JournalEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		timestamp := parts[0]
		if sec, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
			timestamp = time.Unix(sec, 0).Format("15:04:05")
		}
		cmd := parts[1]
		entries = append(entries, ui.JournalEntry{Timestamp: timestamp, Command: cmd, Tags: tagsForCommand(cmd)})
	}
	return entries
}

func tagsForCommand(cmd string) []string {
	out := []string{}
	if strings.Contains(cmd, "|") {
		out = append(out, "pipe")
	}
	if regexp.MustCompile(`\bfind\b`).MatchString(cmd) {
		out = append(out, "find")
	}
	if strings.Contains(cmd, "-print0") || strings.Contains(cmd, "xargs -0") {
		out = append(out, "null-safe")
	}
	return out
}

func (a *App) advanceLevel() {
	if len(a.pack.LoadedLevels) == 0 {
		return
	}
	idx := 0
	for i, lv := range a.pack.LoadedLevels {
		if lv.LevelID == a.level.LevelID {
			idx = i
			break
		}
	}
	a.level = a.pack.LoadedLevels[(idx+1)%len(a.pack.LoadedLevels)]
}

func (a *App) setDevState(state, demo string) {
	a.devMu.Lock()
	defer a.devMu.Unlock()
	a.devState.State = state
	a.devState.Demo = demo
	a.devState.RenderSeq++
}

func (a *App) getDevState() map[string]any {
	a.devMu.Lock()
	defer a.devMu.Unlock()
	return map[string]any{
		"ok":         true,
		"state":      a.devState.State,
		"demo":       a.devState.Demo,
		"render_seq": a.devState.RenderSeq,
	}
}

func (a *App) startDevHTTP() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/__dev/ready", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a.getDevState())
	})
	mux.HandleFunc("/__dev/demo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Demo string `json:"demo"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid json"})
			return
		}
		req.Demo = strings.TrimSpace(req.Demo)
		if req.Demo == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "demo is required"})
			return
		}
		a.applyDemoScenario(context.Background(), req.Demo)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a.getDevState())
	})

	a.devServer = &http.Server{Addr: a.cfg.DevHTTP, Handler: mux}
	a.setDevState("playing", a.cfg.DemoScenario)
	go func() {
		if err := a.devServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("dev_http.listen_failed", map[string]any{"error": err.Error(), "addr": a.cfg.DevHTTP})
		}
	}()
	return nil
}

func resultSummary(passed bool) string {
	if passed {
		return "All required checks passed."
	}
	return "Some required checks failed."
}

func currentScore(a *App) int {
	if a.lastResult.Score.TotalPoints > 0 {
		return a.lastResult.Score.TotalPoints
	}
	if a.level.Scoring.BasePoints > 0 {
		return a.level.Scoring.BasePoints
	}
	return 1000
}

func (a *App) badgesFor(passed bool) []string {
	if !passed {
		return nil
	}
	b := []string{}
	cmdLog := filepath.Join(a.handle.WorkDir(), ".dojo_cmdlog")
	body, err := os.ReadFile(cmdLog)
	if err == nil {
		if !regexp.MustCompile(`\bcat\s+\S+\s+\|`).Match(body) {
			b = append(b, "No Useless Cat")
		}
		if strings.Contains(string(body), " -print0") || strings.Contains(string(body), "xargs -0") {
			b = append(b, "Whitespace Warrior")
		}
	}
	sort.Strings(b)
	return b
}

func containerName(sessionID, levelID string) string {
	safe := regexp.MustCompile(`[^a-zA-Z0-9_.-]`).ReplaceAllString(levelID, "_")
	short := sessionID
	if len(short) > 8 {
		short = short[:8]
	}
	return "clidojo_" + short + "_" + safe
}

func ifThenElse(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
