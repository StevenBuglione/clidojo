package app

import "strings"

type GameMode string

const (
	ModeFreePlay   GameMode = "free"
	ModeDailyDrill GameMode = "daily"
	ModeCampaign   GameMode = "campaign"
)

func normalizeGameMode(raw string) GameMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ModeDailyDrill), "dailydrill":
		return ModeDailyDrill
	case string(ModeCampaign):
		return ModeCampaign
	default:
		return ModeFreePlay
	}
}
