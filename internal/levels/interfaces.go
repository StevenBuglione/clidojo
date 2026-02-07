package levels

import "context"

type Loader interface {
	LoadPacks(ctx context.Context, root string) ([]Pack, error)
	FindLevel(packs []Pack, packID string, levelID string) (Pack, Level, error)
	StageWorkdir(level Level, workdir string) error
}
