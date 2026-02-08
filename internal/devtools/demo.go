package devtools

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clidojo/internal/grading"
	"clidojo/internal/term"
)

type Scenario struct {
	Name        string
	MenuOpen    bool
	HintsOpen   bool
	GoalOpen    bool
	JournalOpen bool
	ResultPass  *bool
}

type Manager struct{}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Resolve(name string) Scenario {
	pass := true
	fail := false
	switch name {
	case "main_menu":
		return Scenario{Name: name}
	case "level_select":
		return Scenario{Name: name}
	case "menu":
		return Scenario{Name: "pause_menu", MenuOpen: true}
	case "pause_menu":
		return Scenario{Name: name, MenuOpen: true}
	case "playing", "playable":
		return Scenario{Name: name}
	case "results_pass":
		return Scenario{Name: name, ResultPass: &pass}
	case "results_fail":
		return Scenario{Name: name, ResultPass: &fail}
	case "hints_open":
		return Scenario{Name: name, HintsOpen: true}
	case "journal_open":
		return Scenario{Name: name, JournalOpen: true}
	default:
		return Scenario{Name: "playing"}
	}
}

func (m *Manager) PlaybackFrames(levelID, scenario string) []term.PlaybackFrame {
	if scenario == "playable" {
		scenario = "playing"
	}
	if scenario == "pause_menu" {
		scenario = "menu"
	}
	keys := []string{
		fmt.Sprintf("%s_%s", scenario, levelID),
		scenario,
	}
	for _, key := range keys {
		b64, ok := prerecordedTTYRecBase64[key]
		if !ok {
			continue
		}
		frames, err := decodeTTYRecBase64(b64)
		if err == nil && len(frames) > 0 {
			return frames
		}
	}

	// Conservative fallback if fixture decoding fails.
	return []term.PlaybackFrame{
		{After: 0, Data: []byte("\x1b[2J\x1b[HCLI Dojo mock sandbox\r\n")},
		{After: 40 * time.Millisecond, Data: []byte(fmt.Sprintf("Level: %s\r\n", levelID))},
		{After: 40 * time.Millisecond, Data: []byte("\x1b[32mplayer@dojo:/work$ \x1b[0m")},
	}
}

func (m *Manager) MockCmdLog(levelID string) string {
	switch levelID {
	case "level-001-pipes-101":
		return "1700000001\tsort /levels/current/animals.txt | uniq -c | sort -nr | awk '{print $1 \"\\t\" $2}' > /work/animal_counts.txt\n"
	case "level-002-find-safe":
		return "1700000001\tfind /levels/current/logs -type f -name '*.log' -print0 | xargs -0 grep -h 'ERROR' | wc -l > /work/error_lines.txt\n"
	case "level-003-top-ips":
		return "1700000001\tawk '{print $1}' /levels/current/access.log | sort | uniq -c | sort -nr | head -n 5 | awk '{print $1 \" \" $2}' > /work/top_ips.txt\n"
	default:
		return "1700000001\techo ready\n"
	}
}

func (m *Manager) MockGrade(req MockGradeRequest) grading.Result {
	passed := req.Attempt >= 2

	checks := make([]grading.CheckResult, 0, len(req.Checks))
	artifacts := make([]grading.Artifact, 0)
	firstFailureUsed := false
	for _, c := range req.Checks {
		checkPass := true
		message := "ok"
		if !passed && c.Required && !firstFailureUsed {
			firstFailureUsed = true
			checkPass = false
			message = firstNonEmpty(c.OnFailMessage, "deterministic mock failure")
			artifacts = append(artifacts, grading.Artifact{
				Ref:         "diff_" + c.ID,
				Kind:        "unified_diff",
				Title:       c.ID,
				TextPreview: "--- expected\n+++ actual\n- expected line\n+ actual line\n",
			})
		}
		checks = append(checks, grading.CheckResult{
			ID:       c.ID,
			Type:     c.Type,
			Required: c.Required,
			Passed:   checkPass,
			Summary:  message,
			Message:  message,
		})
	}

	base := defaultInt(req.BasePoints, 1000)
	grace := defaultInt(req.GraceSeconds, 60)
	timePenalty := defaultInt(req.TimePenalty, 1)
	hintPenalty := defaultInt(req.HintPenalty, 80)
	resetPenalty := defaultInt(req.ResetPenalty, 120)

	timePenaltyPoints := 0
	if req.ElapsedSeconds > grace {
		timePenaltyPoints = (req.ElapsedSeconds - grace) * timePenalty
	}
	hintPenaltyPoints := req.HintsUsed * hintPenalty
	resetPenaltyPoints := req.Resets * resetPenalty
	total := base - timePenaltyPoints - hintPenaltyPoints - resetPenaltyPoints
	if !passed {
		total -= 50
	}
	if total < 0 {
		total = 0
	}

	now := time.Now()
	result := grading.Result{
		Kind:          grading.ResultKind,
		SchemaVersion: grading.SchemaVersion,
		PackID:        req.PackID,
		PackVersion:   req.PackVersion,
		LevelID:       req.LevelID,
		Run: grading.RunInfo{
			RunID:            "mock-run",
			Attempt:          max(1, req.Attempt),
			StartedAtUnixMS:  now.Add(-2 * time.Second).UnixMilli(),
			FinishedAtUnixMS: now.UnixMilli(),
			DurationMS:       2000,
		},
		Passed:    passed,
		Checks:    checks,
		Artifacts: artifacts,
		Score: grading.Score{
			BasePoints:          base,
			TimeGraceSeconds:    grace,
			TimePenaltyPoints:   timePenaltyPoints,
			HintPenaltyPoints:   hintPenaltyPoints,
			ResetPenaltyPoints:  resetPenaltyPoints,
			OptionalBonusPoints: 0,
			TotalPoints:         total,
			Breakdown: []grading.ScoreDelta{
				{Kind: "time", Points: -timePenaltyPoints, Description: "Time penalty after grace"},
				{Kind: "hint", Points: -hintPenaltyPoints, Description: "Hints revealed"},
				{Kind: "reset", Points: -resetPenaltyPoints, Description: "Resets used"},
			},
		},
		EngineDebug: grading.EngineDebug{Engine: "mock", ContainerName: "mock", ImageRef: "mock"},
	}
	return result
}

func (m *Manager) SetState(ctx context.Context, cacheDir string, state string, rendered bool) error {
	_ = ctx
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		cacheDir = filepath.Join(home, ".cache", "clidojo")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	payload := map[string]any{
		"state":    strings.TrimSpace(state),
		"rendered": rendered,
	}
	b, _ := json.Marshal(payload)
	return os.WriteFile(filepath.Join(cacheDir, "dev_state.json"), b, 0o644)
}

func defaultInt(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}

func max(a, b int) int {
	if a > b {
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

func decodeTTYRecBase64(s string) ([]term.PlaybackFrame, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, err
	}
	return decodeTTYRec(data)
}

func decodeTTYRec(data []byte) ([]term.PlaybackFrame, error) {
	if len(data) < 12 {
		return nil, errors.New("ttyrec data too short")
	}

	frames := make([]term.PlaybackFrame, 0, 16)
	offset := 0
	var lastTS int64
	first := true

	for {
		if offset == len(data) {
			break
		}
		if offset+12 > len(data) {
			return nil, errors.New("truncated ttyrec header")
		}

		sec := binary.LittleEndian.Uint32(data[offset : offset+4])
		usec := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		size := binary.LittleEndian.Uint32(data[offset+8 : offset+12])
		offset += 12

		if size > uint32(len(data)-offset) {
			return nil, errors.New("truncated ttyrec payload")
		}
		chunk := append([]byte(nil), data[offset:offset+int(size)]...)
		offset += int(size)

		tsUS := int64(sec)*1_000_000 + int64(usec)
		delay := time.Duration(0)
		if !first {
			delta := tsUS - lastTS
			if delta > 0 {
				delay = time.Duration(delta) * time.Microsecond
			}
		}
		first = false
		lastTS = tsUS

		frames = append(frames, term.PlaybackFrame{
			After: delay,
			Data:  chunk,
		})
	}

	if len(frames) == 0 {
		return nil, errors.New("no frames in ttyrec")
	}
	return frames, nil
}

var prerecordedTTYRecBase64 = map[string]string{
	"menu":                        "APFTZQAAAAAeAAAAG1syShtbSENMSSBEb2pvIG1vY2sgc2FuZGJveA0KAPFTZUCcAAAiAAAAUHJlc3MgRjEwIGZvciBtZW51LCBGNSB0byBjaGVjay4NCgDxU2WAOAEAHAAAABtbMzJtcGxheWVyQGRvam86L3dvcmskIBtbMG0=",
	"playing_level-001-pipes-101": "APFTZQAAAAA4AAAAG1syShtbSFdlbGNvbWUgdG8gQ0xJIERvam8NCkxldmVsOiBsZXZlbC0wMDEtcGlwZXMtMTAxDQoA8VNlQJwAABwAAAAbWzMybXBsYXllckBkb2pvOi93b3JrJCAbWzBtAPFTZYA4AQBsAAAAc29ydCAvbGV2ZWxzL2N1cnJlbnQvYW5pbWFscy50eHQgfCB1bmlxIC1jIHwgc29ydCAtbnIgfCBhd2sgJ3twcmludCAkMSAiXHQiICQyfScgPiAvd29yay9hbmltYWxfY291bnRzLnR4dA0KAPFTZcDUAQAcAAAAG1szMm1wbGF5ZXJAZG9qbzovd29yayQgG1swbQ==",
	"playing_level-002-find-safe": "APFTZQAAAAA4AAAAG1syShtbSFdlbGNvbWUgdG8gQ0xJIERvam8NCkxldmVsOiBsZXZlbC0wMDItZmluZC1zYWZlDQoA8VNlQJwAABwAAAAbWzMybXBsYXllckBkb2pvOi93b3JrJCAbWzBtAPFTZYA4AQB0AAAAZmluZCAvbGV2ZWxzL2N1cnJlbnQvbG9ncyAtdHlwZSBmIC1uYW1lICcqLmxvZycgLXByaW50MCB8IHhhcmdzIC0wIGdyZXAgLWggJ0VSUk9SJyB8IHdjIC1sID4gL3dvcmsvZXJyb3JfbGluZXMudHh0DQoA8VNlwNQBABwAAAAbWzMybXBsYXllckBkb2pvOi93b3JrJCAbWzBt",
	"playing_level-003-top-ips":   "APFTZQAAAAA2AAAAG1syShtbSFdlbGNvbWUgdG8gQ0xJIERvam8NCkxldmVsOiBsZXZlbC0wMDMtdG9wLWlwcw0KAPFTZUCcAAAcAAAAG1szMm1wbGF5ZXJAZG9qbzovd29yayQgG1swbQDxU2WAOAEAgwAAAGF3ayAne3ByaW50ICQxfScgL2xldmVscy9jdXJyZW50L2FjY2Vzcy5sb2cgfCBzb3J0IHwgdW5pcSAtYyB8IHNvcnQgLW5yIHwgaGVhZCAtbiA1IHwgYXdrICd7cHJpbnQgJDEgIiAiICQyfScgPiAvd29yay90b3BfaXBzLnR4dA0KAPFTZcDUAQAcAAAAG1szMm1wbGF5ZXJAZG9qbzovd29yayQgG1swbQ==",
	"results_pass":                "APFTZQAAAAAmAAAAG1syShtbSFdlbGNvbWUgdG8gQ0xJIERvam8NClBBU1MgcnVuDQoA8VNlQJwAABwAAAAbWzMybXBsYXllckBkb2pvOi93b3JrJCAbWzBtAPFTZYA4AQBVAAAAYXdrICd7cHJpbnQgJDF9JyAvbGV2ZWxzL2N1cnJlbnQvYWNjZXNzLmxvZyB8IHNvcnQgfCB1bmlxIC1jIHwgc29ydCAtbnIgfCBoZWFkIC1uIDUNCgDxU2XA1AEAPAAAADQgMTAuMC4wLjYNCjMgMTAuMC4wLjENCjMgMTAuMC4wLjQNCjMgMTAuMC4wLjINCjIgMTAuMC4wLjUNCgDxU2UAcQIAHAAAABtbMzJtcGxheWVyQGRvam86L3dvcmskIBtbMG0=",
	"results_fail":                "APFTZQAAAAAmAAAAG1syShtbSFdlbGNvbWUgdG8gQ0xJIERvam8NCkZBSUwgcnVuDQoA8VNlQJwAABwAAAAbWzMybXBsYXllckBkb2pvOi93b3JrJCAbWzBtAPFTZYA4AQBCAAAAY2F0IC9sZXZlbHMvY3VycmVudC9hbmltYWxzLnR4dCB8IHNvcnQgPiAvd29yay9hbmltYWxfY291bnRzLnR4dA0KAPFTZcDUAQAiAAAAKGNoZWNrIGZhaWxlZDogY29udGVudCBtaXNtYXRjaCkNCgDxU2UAcQIAHAAAABtbMzJtcGxheWVyQGRvam86L3dvcmskIBtbMG0=",
	"hints_open":                  "APFTZQAAAAAoAAAAG1syShtbSFdlbGNvbWUgdG8gQ0xJIERvam8NCkhpbnRzIGRlbW8NCgDxU2VAnAAAHAAAABtbMzJtcGxheWVyQGRvam86L3dvcmskIBtbMG0A8VNlgDgBADEAAABmaW5kIC9sZXZlbHMvY3VycmVudC9sb2dzIC1uYW1lICcqLmxvZycgLXR5cGUgZg0KAPFTZcDUAQAcAAAAG1szMm1wbGF5ZXJAZG9qbzovd29yayQgG1swbQ==",
	"journal_open":                "APFTZQAAAAAqAAAAG1syShtbSFdlbGNvbWUgdG8gQ0xJIERvam8NCkpvdXJuYWwgZGVtbw0KAPFTZUCcAAAcAAAAG1szMm1wbGF5ZXJAZG9qbzovd29yayQgG1swbQDxU2WAOAEAVAAAAGZpbmQgL2xldmVscy9jdXJyZW50L2xvZ3MgLXR5cGUgZiAtbmFtZSAnKi5sb2cnIC1wcmludDAgfCB4YXJncyAtMCBncmVwIC1oICdFUlJPUicNCgDxU2XA1AEAHAAAABtbMzJtcGxheWVyQGRvam86L3dvcmskIBtbMG0=",
}
