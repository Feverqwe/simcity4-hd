package macos

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Feverqwe/simcity4-hd/internal/sc4/exepatch/macho"
	gamefiles "github.com/Feverqwe/simcity4-hd/internal/sc4/game"
	"github.com/Feverqwe/simcity4-hd/internal/sc4/setup"
)

const macSetupReceipt = "Installed.json"

type macSetupSources struct {
	UI                string         `json:"ui,omitempty"`
	World             string         `json:"world,omitempty"`
	Cache             string         `json:"cache,omitempty"`
	Build             *macSetupBuild `json:"build,omitempty"`
	PendingResolution string         `json:"pendingResolution,omitempty"`
	reusedUI          macInstalledInterface
	world             macWorldPlan
	resolution        int

	overrideRegionMap, overrideLoadingPreview, overrideCursors, overrideLanguage bool
}

type macSetupRecord struct {
	macSetupSources
	ExecutableSHA256 string   `json:"executableSHA256"`
	WorldFiles       []string `json:"worldFiles,omitempty"`
	NeedsSigning     bool     `json:"needsSigning,omitempty"`
}

type macSetupBuild struct {
	Scale              int    `json:"scale"`
	Language           string `json:"language"`
	Interface          bool   `json:"interface"`
	Cursors            bool   `json:"cursors"`
	RegionMap          bool   `json:"regionMap"`
	LoadingPreview     bool   `json:"loadingPreview"`
	RetinaText         bool   `json:"retinaText,omitempty"`
	World              bool   `json:"world"`
	Network            bool   `json:"network"`
	Terrain            bool   `json:"terrain"`
	Models             bool   `json:"models"`
	SkipIntro          *bool  `json:"skipIntro,omitempty"`
	DrawStats          bool   `json:"drawStats"`
	KeepUpscaledImages bool   `json:"keepUpscaledImages,omitempty"`
}

func macBuildOptions(command setup.Command) *macSetupBuild {
	return &macSetupBuild{
		Scale: command.Scale, Language: command.Language, Interface: command.Interface,
		Cursors: command.Features.HdCursors, RegionMap: command.Features.RegionMap,
		LoadingPreview: command.Features.LoadingPreview, RetinaText: command.Interface && command.RetinaText,
		World: command.World, Network: command.Categories.Network,
		Terrain: command.Categories.Terrain, Models: command.Categories.Models,
		SkipIntro: &command.SkipIntro, DrawStats: command.DrawStats,
		KeepUpscaledImages: command.KeepUpscaledImages,
	}
}

func (b macSetupBuild) skipIntro() bool { return b.SkipIntro == nil || *b.SkipIntro }

func macSetupReceiptPath(bundle string) string {
	return filepath.Join(macModPlugins(bundle), macSetupReceipt)
}

func macModPlugins(bundle string) string {
	return filepath.Join(bundle, "Contents", "GameData", "Plugins", "SimCity4HD")
}

func macSetupBackup(bundle string) string {
	return filepath.Join(filepath.Dir(bundle), "."+filepath.Base(bundle)+".sc4-uhd-original")
}

func readMacSetupReceipt(bundle string) (record macSetupRecord, err error) {
	data, err := os.ReadFile(macSetupReceiptPath(bundle))
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	if (record.UI == "" && record.World == "" && record.Build == nil) || record.ExecutableSHA256 == "" {
		return record, errors.New("the macOS mod receipt is incomplete")
	}
	return record, nil
}

func writeMacSetupReceipt(bundle string, sources macSetupSources, world []string, needsSigning bool) error {
	hash, err := macSetupExecutableHash(bundle)
	if err != nil {
		return err
	}
	record := macSetupRecord{macSetupSources: sources, ExecutableSHA256: hash, NeedsSigning: needsSigning}
	for _, path := range world {
		record.WorldFiles = append(record.WorldFiles, filepath.Base(path))
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(macSetupReceiptPath(bundle), data, 0644)
}

func macSetupExecutableHash(bundle string) (string, error) {
	exe, err := os.ReadFile(gamefiles.ExecutablePath(bundle, true))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(exe)), nil
}

func checkStockMacSetup(bundle string) error {
	hash, err := macSetupExecutableHash(bundle)
	if err != nil {
		return err
	}
	if hash != macho.StockHash {
		return errors.New("Setup requires the original Steam Aspyr 1.2.1 (185949) game executable.")
	}
	return nil
}
