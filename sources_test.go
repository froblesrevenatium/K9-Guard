package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProject(t *testing.T, root, project string, names ...string) string {
	t.Helper()
	dir := filepath.Join(root, project, ".idea")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	shared := `<project version="4"><component name="DataSourceManagerImpl">`
	local := `<project version="4"><component name="dataSourceStorageLocal">`
	for i, name := range names {
		uuid := project + "-" + string(rune('a'+i))
		shared += `<data-source name="` + name + `" uuid="` + uuid + `"><jdbc-url>jdbc:postgresql://localhost:5432/postgres</jdbc-url></data-source>`
		local += `<data-source name="` + name + `" uuid="` + uuid + `"><user-name>postgres</user-name></data-source>`
	}
	shared += `</component></project>`
	local += `</component></project>`
	os.WriteFile(filepath.Join(dir, "dataSources.xml"), []byte(shared), 0o600)
	os.WriteFile(filepath.Join(dir, "dataSources.local.xml"), []byte(local), 0o600)
	return dir
}

func TestConnectionSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DBWATCH_HOME", filepath.Join(home, ".dbwatch"))
	t.Setenv("DBWATCH_DATAGRIP_PROJECT", "")
	projects := filepath.Join(home, "DataGripProjects")
	alpha := writeProject(t, projects, "Alpha", "Localhost", "Stage")
	beta := writeProject(t, projects, "Beta", "Localhost")
	external := writeProject(t, filepath.Join(home, "otros"), "Gamma", "Reportes")

	connections, err := loadConnections()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range connections {
		names[c.Name] = true
	}
	for _, want := range []string{"Localhost (Alpha)", "Localhost (Beta)", "Stage"} {
		if !names[want] {
			t.Fatalf("falta %q en %v", want, names)
		}
	}

	if err := saveSettings(appSettings{Configured: true, EnabledDirs: []string{beta, external}, ExtraProjects: []string{filepath.Dir(external)}}); err != nil {
		t.Fatal(err)
	}
	connections, err = loadConnections()
	if err != nil {
		t.Fatal(err)
	}
	names = map[string]bool{}
	for _, c := range connections {
		names[c.Name] = true
	}
	if len(names) != 2 || !names["Localhost"] || !names["Reportes"] {
		t.Fatalf("con Beta y Gamma activas quería Localhost y Reportes, hay %v", names)
	}
	if _, ok := projectSource(filepath.Dir(alpha)); !ok {
		t.Fatal("projectSource debe aceptar la carpeta del proyecto")
	}
	if _, ok := projectSource(home); ok {
		t.Fatal("una carpeta sin .idea no es un proyecto")
	}
}
