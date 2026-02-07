package levels

import "testing"

func TestPackValidateRejectsUnsupportedSchemaVersion(t *testing.T) {
	p := Pack{
		Kind:          PackKind,
		SchemaVersion: SupportedSchemaVersion + 1,
		PackID:        "builtin-core",
		Name:          "x",
		Version:       "0.1.0",
		Image:         PackImage{Ref: "img"},
	}
	if err := p.Validate(); err == nil {
		t.Fatalf("expected unsupported schema version error")
	}
}

func TestLevelValidateRequiresAtLeastOneRequiredCheck(t *testing.T) {
	required := false
	l := Level{
		Kind:             LevelKind,
		SchemaVersion:    1,
		LevelID:          "level-123",
		Title:            "x",
		Difficulty:       1,
		EstimatedMinutes: 1,
		Filesystem: FilesystemSpec{
			Dataset: DatasetSpec{Source: "dir", Path: "dataset", MountPoint: "/levels/current"},
			Work:    WorkSpec{MountPoint: "/work"},
		},
		Objective: ObjectiveSpec{Bullets: []string{"do thing"}},
		Checks: []CheckSpec{
			{ID: "c1", Type: "file_exists", Description: "desc", Required: &required, Path: "/work/out.txt"},
		},
	}
	if err := l.Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestLevelValidateRejectsRelativeCheckPath(t *testing.T) {
	required := true
	l := Level{
		Kind:             LevelKind,
		SchemaVersion:    1,
		LevelID:          "level-abc",
		Title:            "x",
		Difficulty:       1,
		EstimatedMinutes: 1,
		Filesystem: FilesystemSpec{
			Dataset: DatasetSpec{Source: "dir", Path: "dataset", MountPoint: "/levels/current"},
			Work:    WorkSpec{MountPoint: "/work"},
		},
		Objective: ObjectiveSpec{Bullets: []string{"do thing"}},
		Checks: []CheckSpec{
			{ID: "c1", Type: "file_exists", Description: "desc", Required: &required, Path: "work/out.txt"},
		},
	}
	if err := l.Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
}
