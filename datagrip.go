package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Connection struct {
	Name     string
	UUID     string
	Host     string
	Port     string
	Database string
	User     string
	Source   string
	SSH      *SSHConfig
}

type SSHConfig struct {
	Host    string
	Port    string
	User    string
	KeyPath string
}

type xmlDataSource struct {
	Name    string `xml:"name,attr"`
	UUID    string `xml:"uuid,attr"`
	JdbcURL string `xml:"jdbc-url"`
	User    string `xml:"user-name"`
	SSH     struct {
		Enabled  string `xml:"enabled"`
		ConfigID string `xml:"ssh-config-id"`
	} `xml:"ssh-properties"`
}

type xmlProject struct {
	Components []struct {
		Sources []xmlDataSource `xml:"data-source"`
	} `xml:"component"`
}

type xmlSSHConfigs struct {
	Components []struct {
		Configs []struct {
			ID       string `xml:"id,attr"`
			Host     string `xml:"host,attr"`
			Port     string `xml:"port,attr"`
			Username string `xml:"username,attr"`
			KeyPath  string `xml:"keyPath,attr"`
		} `xml:"configs>sshConfig"`
	} `xml:"component"`
}

func readDataSources(path string) ([]xmlDataSource, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var project xmlProject
	if err := xml.Unmarshal(raw, &project); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var sources []xmlDataSource
	for _, component := range project.Components {
		sources = append(sources, component.Sources...)
	}
	return sources, nil
}

func sshConfigsPath() (string, error) {
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, "Library", "Application Support", "JetBrains", "DataGrip*", "options", "sshConfigs.xml"))
	if len(matches) == 0 {
		return "", fmt.Errorf("no se encontró sshConfigs.xml de DataGrip")
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

func readSSHConfigs() (map[string]SSHConfig, error) {
	path, err := sshConfigsPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var parsed xmlSSHConfigs
	if err := xml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	home, _ := os.UserHomeDir()
	configs := map[string]SSHConfig{}
	for _, component := range parsed.Components {
		for _, c := range component.Configs {
			port := c.Port
			if port == "" {
				port = "22"
			}
			configs[c.ID] = SSHConfig{
				Host:    c.Host,
				Port:    port,
				User:    c.Username,
				KeyPath: strings.ReplaceAll(c.KeyPath, "$USER_HOME$", home),
			}
		}
	}
	return configs, nil
}

func parseJdbcURL(jdbc string) (host, port, database string, err error) {
	parsed, err := url.Parse(strings.TrimPrefix(jdbc, "jdbc:"))
	if err != nil {
		return "", "", "", err
	}
	host = parsed.Hostname()
	port = parsed.Port()
	if port == "" {
		port = "5432"
	}
	database = strings.TrimPrefix(parsed.Path, "/")
	return host, port, database, nil
}

func latestDataGripOptionsDir() string {
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, "Library", "Application Support", "JetBrains", "DataGrip*", "options"))
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return matches[len(matches)-1]
}

func readOptionalDataSources(path string) []xmlDataSource {
	sources, err := readDataSources(path)
	if err != nil {
		return nil
	}
	return sources
}

func sourceConnections(source connectionSource, sshConfigs map[string]SSHConfig, sshErr error) ([]Connection, error) {
	shared := readOptionalDataSources(filepath.Join(source.Dir, "dataSources.xml"))
	localByUUID := map[string]xmlDataSource{}
	for _, l := range readOptionalDataSources(filepath.Join(source.Dir, "dataSources.local.xml")) {
		localByUUID[l.UUID] = l
	}
	var connections []Connection
	for _, s := range shared {
		host, port, database, err := parseJdbcURL(s.JdbcURL)
		if err != nil || host == "" {
			continue
		}
		l := localByUUID[s.UUID]
		connection := Connection{Name: s.Name, UUID: s.UUID, Host: host, Port: port, Database: database, User: l.User, Source: source.Name}
		if l.SSH.Enabled == "true" {
			if sshErr != nil {
				return nil, sshErr
			}
			ssh, ok := sshConfigs[l.SSH.ConfigID]
			if !ok {
				return nil, fmt.Errorf("la conexión %s de %s usa un túnel SSH que no está en sshConfigs.xml", s.Name, source.Name)
			}
			connection.SSH = &ssh
		}
		connections = append(connections, connection)
	}
	return connections, nil
}

func loadConnections() ([]Connection, error) {
	sources := activeSources()
	if len(sources) == 0 {
		return nil, errors.New("no se encontró ningún proyecto de DataGrip; agrega uno en Fuentes de conexiones")
	}
	sshConfigs, sshErr := readSSHConfigs()
	var connections []Connection
	for _, source := range sources {
		found, err := sourceConnections(source, sshConfigs, sshErr)
		if err != nil {
			return nil, err
		}
		connections = append(connections, found...)
	}
	counts := map[string]int{}
	for _, c := range connections {
		counts[strings.ToLower(c.Name)]++
	}
	for i := range connections {
		if counts[strings.ToLower(connections[i].Name)] > 1 {
			connections[i].Name = connections[i].Name + " (" + connections[i].Source + ")"
		}
	}
	return connections, nil
}

func findConnection(name string) (Connection, error) {
	connections, err := loadConnections()
	if err != nil {
		return Connection{}, err
	}
	for _, c := range connections {
		if strings.EqualFold(c.Name, name) {
			return c, nil
		}
	}
	return Connection{}, fmt.Errorf("no existe la conexión %q en DataGrip", name)
}

func readKeychainPassword(uuid string) (string, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", "IntelliJ Platform DB — "+uuid, "-w").Output()
	if err != nil {
		return "", fmt.Errorf("no se encontró la contraseña en el llavero de macOS; en DataGrip guarda la contraseña y usa Settings → Appearance & Behavior → System Settings → Passwords → In native Keychain")
	}
	return strings.TrimRight(string(out), "\n"), nil
}

var (
	passwordMu    sync.Mutex
	passwordCache = map[string]string{}
)

func keychainPassword(uuid string) (string, error) {
	passwordMu.Lock()
	defer passwordMu.Unlock()
	if password, ok := passwordCache[uuid]; ok {
		return password, nil
	}
	password, err := readKeychainPassword(uuid)
	if err != nil {
		return "", err
	}
	passwordCache[uuid] = password
	return password, nil
}
