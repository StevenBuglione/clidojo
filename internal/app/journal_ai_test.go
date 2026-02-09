package app

import (
	"strings"
	"testing"

	"clidojo/internal/levels"
)

func TestSplitPipelineStagesKeepsQuotedPipes(t *testing.T) {
	command := `awk '{print $1 "|" $2}' /tmp/in.txt | sort -nr | head -n 5`
	stages := splitPipelineStages(command)
	if len(stages) != 3 {
		t.Fatalf("expected 3 stages, got %d (%#v)", len(stages), stages)
	}
	if !strings.Contains(stages[0], `print $1 "|" $2`) {
		t.Fatalf("first stage lost quoted pipe: %q", stages[0])
	}
}

func TestBuildJournalExplainTextAddsCoachingFromFailedChecks(t *testing.T) {
	level := levels.Level{
		Objective: levels.ObjectiveSpec{
			Bullets: []string{"Create /work/animal_counts.txt"},
		},
	}
	status := map[string]string{
		"output_format_tab":    "fail",
		"content_counts_match": "fail",
	}
	text := buildJournalExplainText(
		`sort animals.txt | uniq -c | sort -nr | awk '{print $1 "\t" $2}' > /work/animal_counts.txt`,
		level,
		status,
		false,
	)
	if !strings.Contains(text, "Level coaching") {
		t.Fatalf("expected coaching section, got: %s", text)
	}
	if !strings.Contains(text, "tab-separated") {
		t.Fatalf("expected tab formatting hint, got: %s", text)
	}
	if !strings.Contains(text, "Objective reminder") {
		t.Fatalf("expected objective reminder, got: %s", text)
	}
}

func TestBuildJournalExplainTextAddsSuccessNudgeOnPass(t *testing.T) {
	text := buildJournalExplainText(`echo ok`, levels.Level{}, map[string]string{}, true)
	if !strings.Contains(text, "Nice run") {
		t.Fatalf("expected success nudge, got: %s", text)
	}
}
