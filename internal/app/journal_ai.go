package app

import (
	"fmt"
	"strings"

	"clidojo/internal/levels"
)

func buildJournalExplainText(command string, level levels.Level, checkStatus map[string]string, passed bool) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "No command to explain."
	}

	stages := splitPipelineStages(trimmed)
	if len(stages) == 0 {
		stages = []string{trimmed}
	}

	var b strings.Builder
	b.WriteString("Command\n")
	b.WriteString(trimmed)
	b.WriteString("\n\nWhat this does\n")
	for i, stage := range stages {
		name := stageCommandName(stage)
		desc := describeCommandStage(name)
		if desc == "" {
			desc = "Runs this stage in the shell."
		}
		b.WriteString(fmt.Sprintf("%d. `%s` - %s\n", i+1, stage, desc))
	}

	if hint := pipelineOrderingHint(stages); hint != "" {
		b.WriteString("\nPipeline hint\n")
		b.WriteString("- " + hint + "\n")
	}
	if redir := redirectionHint(trimmed); redir != "" {
		b.WriteString("- " + redir + "\n")
	}

	coach := checkBasedCoaching(checkStatus)
	if len(coach) > 0 {
		b.WriteString("\nLevel coaching\n")
		for _, line := range coach {
			b.WriteString("- " + line + "\n")
		}
	} else if passed {
		b.WriteString("\nLevel coaching\n- Nice run. You can now optimize for shorter commands or better readability.\n")
	}

	if len(level.Objective.Bullets) > 0 {
		b.WriteString("\nObjective reminder\n")
		for _, objective := range level.Objective.Bullets {
			b.WriteString("- " + strings.TrimSpace(objective) + "\n")
		}
	}

	return strings.TrimSpace(b.String())
}

func splitPipelineStages(command string) []string {
	var out []string
	var buf strings.Builder
	var quote byte
	escaped := false
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			buf.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			buf.WriteByte(ch)
			continue
		}
		if quote != 0 {
			buf.WriteByte(ch)
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			buf.WriteByte(ch)
			continue
		}
		if ch == '|' {
			stage := strings.TrimSpace(buf.String())
			if stage != "" {
				out = append(out, stage)
			}
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	stage := strings.TrimSpace(buf.String())
	if stage != "" {
		out = append(out, stage)
	}
	return out
}

func stageCommandName(stage string) string {
	fields := strings.Fields(stage)
	if len(fields) == 0 {
		return ""
	}
	i := 0
	for i < len(fields) {
		token := fields[i]
		if strings.Contains(token, "=") && !strings.HasPrefix(token, "-") && !strings.Contains(token, "/") {
			if parts := strings.SplitN(token, "=", 2); len(parts) == 2 && parts[0] != "" {
				i++
				continue
			}
		}
		break
	}
	if i >= len(fields) {
		return ""
	}
	name := fields[i]
	if name == "sudo" || name == "command" || name == "time" {
		i++
		if i >= len(fields) {
			return ""
		}
		name = fields[i]
	}
	return strings.TrimSpace(name)
}

func describeCommandStage(name string) string {
	switch name {
	case "sort":
		return "Sorts lines; use `-n` for numeric sort and `-r` for descending."
	case "uniq":
		return "`uniq -c` counts adjacent duplicates, so input should usually be sorted first."
	case "awk":
		return "Processes fields (`$1`, `$2`, ...); `\"\\t\"` writes a tab separator."
	case "grep":
		return "Filters lines that match a pattern."
	case "find":
		return "Walks directories and emits matching paths."
	case "xargs":
		return "Builds command invocations from stdin items."
	case "tr":
		return "Translates or squeezes characters."
	case "sed":
		return "Applies stream text edits with pattern rules."
	case "cut":
		return "Extracts selected columns or delimiters."
	case "head":
		return "Keeps only the first N lines."
	case "tail":
		return "Keeps only the last N lines."
	case "wc":
		return "Counts lines, words, or bytes."
	case "cat":
		return "Prints file contents; often optional in pipelines."
	default:
		return ""
	}
}

func pipelineOrderingHint(stages []string) string {
	hasUniqCount := false
	seenSortBeforeUniq := false
	seenSort := false
	for _, stage := range stages {
		name := stageCommandName(stage)
		if name == "sort" {
			seenSort = true
		}
		if name == "uniq" && strings.Contains(stage, "-c") {
			hasUniqCount = true
			if seenSort {
				seenSortBeforeUniq = true
			}
		}
	}
	if hasUniqCount && !seenSortBeforeUniq {
		return "Place `sort` before `uniq -c` so equal lines are grouped before counting."
	}
	return ""
}

func redirectionHint(command string) string {
	if idx := strings.Index(command, ">>"); idx >= 0 {
		return "Using `>>` appends output; use `>` if you need to overwrite the file each run."
	}
	if idx := strings.Index(command, ">"); idx >= 0 {
		return "Output is redirected to a file; verify both content and formatting with `cat -vet`."
	}
	return ""
}

func checkBasedCoaching(status map[string]string) []string {
	if len(status) == 0 {
		return nil
	}
	var out []string
	if hasFailedCheck(status, "output_format_tab", "out_format") {
		out = append(out, "Your output format check is failing: ensure each line is tab-separated (`COUNT<TAB>VALUE`).")
	}
	if hasFailedCheck(status, "content_counts_match", "out_matches_expected", "out_exact") {
		out = append(out, "Content mismatch usually means sort/count order or whitespace differs from expected.")
	}
	if hasFailedCheck(status, "used_find") {
		out = append(out, "Use `find` directly in the command pipeline to satisfy the find usage check.")
	}
	if hasFailedCheck(status, "forbid_unsafe_find_loop") {
		out = append(out, "Avoid `for f in $(find ...)`; prefer `find ... -print0 | xargs -0` for safe filenames.")
	}
	if hasFailedCheck(status, "out_lines_5") {
		out = append(out, "Ensure exactly 5 lines (for example with `head -n 5` after sorting).")
	}
	return out
}

func hasFailedCheck(status map[string]string, ids ...string) bool {
	for _, id := range ids {
		if status[id] == "fail" {
			return true
		}
	}
	return false
}
