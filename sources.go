package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type connectionSource struct {
	Name   string
	Dir    string
	Global bool
}

type appSettings struct {
	Configured    bool     `json:"configured"`
	EnabledDirs   []string `json:"enabledSources"`
	ExtraProjects []string `json:"extraProjects"`
}

func settingsPath() string { return filepath.Join(dataDir(), "settings.json") }

func loadSettings() appSettings {
	var settings appSettings
	readJSON(settingsPath(), &settings)
	return settings
}

func saveSettings(settings appSettings) error { return writeJSON(settingsPath(), settings) }

func projectSource(dir string) (connectionSource, bool) {
	dir = filepath.Clean(dir)
	if filepath.Base(dir) != ".idea" {
		dir = filepath.Join(dir, ".idea")
	}
	if _, err := os.Stat(filepath.Join(dir, "dataSources.xml")); err != nil {
		return connectionSource{}, false
	}
	return connectionSource{Name: filepath.Base(filepath.Dir(dir)), Dir: dir}, true
}

func discoverSources(settings appSettings) []connectionSource {
	if dir := os.Getenv("DBWATCH_DATAGRIP_PROJECT"); dir != "" {
		if source, ok := projectSource(dir); ok {
			return []connectionSource{source}
		}
		return nil
	}
	seen := map[string]bool{}
	var sources []connectionSource
	add := func(source connectionSource) {
		if !seen[source.Dir] {
			seen[source.Dir] = true
			sources = append(sources, source)
		}
	}
	home, _ := os.UserHomeDir()
	projects, _ := filepath.Glob(filepath.Join(home, "DataGripProjects", "*", ".idea"))
	sort.Strings(projects)
	for _, dir := range projects {
		if source, ok := projectSource(dir); ok {
			add(source)
		}
	}
	for _, dir := range settings.ExtraProjects {
		if source, ok := projectSource(dir); ok {
			add(source)
		}
	}
	if options := latestDataGripOptionsDir(); options != "" {
		if _, err := os.Stat(filepath.Join(options, "dataSources.xml")); err == nil {
			add(connectionSource{Name: "Globales de DataGrip", Dir: options, Global: true})
		}
	}
	return sources
}

func activeSources() []connectionSource {
	settings := loadSettings()
	sources := discoverSources(settings)
	if !settings.Configured || os.Getenv("DBWATCH_DATAGRIP_PROJECT") != "" {
		return sources
	}
	var active []connectionSource
	for _, source := range sources {
		if contains(settings.EnabledDirs, source.Dir) {
			active = append(active, source)
		}
	}
	return active
}

func sourceLabel(source connectionSource) string {
	home, _ := os.UserHomeDir()
	return source.Name + "  ·  " + strings.Replace(source.Dir, home, "~", 1)
}
