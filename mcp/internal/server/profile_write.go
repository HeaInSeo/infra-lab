package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type profileWriteEnvelope struct {
	OK              bool             `json:"ok"`
	Command         string           `json:"command"`
	ContractVersion string           `json:"contractVersion"`
	Data            profileWriteData `json:"data"`
	Warnings        []any            `json:"warnings"`
	Errors          []any            `json:"errors"`
}

type profileWriteData struct {
	OperationID string          `json:"operationId"`
	Profile     profileWriteRef `json:"profile"`
	Validation  json.RawMessage `json:"validation"`
	AuditPath   string          `json:"auditPath"`
}

type profileWriteRef struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type profileYAML struct {
	Name       string        `yaml:"name"`
	Backend    string        `yaml:"backend"`
	VM         profileVM     `yaml:"vm"`
	Kubernetes profileK8s    `yaml:"kubernetes"`
	Addons     profileAddons `yaml:"addons"`
	State      profileState  `yaml:"state"`
	Libvirt    any           `yaml:"libvirt,omitempty"`
}

type profileVM struct {
	OSImage  string      `yaml:"osImage"`
	ImageURL string      `yaml:"imageUrl,omitempty"`
	Masters  int         `yaml:"masters"`
	Workers  int         `yaml:"workers"`
	Master   profileNode `yaml:"master"`
	Worker   profileNode `yaml:"worker"`
}

type profileNode struct {
	CPU    int    `yaml:"cpu"`
	Memory string `yaml:"memory"`
	Disk   string `yaml:"disk"`
}

type profileK8s struct {
	Version string `yaml:"version"`
	CNI     string `yaml:"cni"`
}

type profileAddons struct {
	Base     []string `yaml:"base"`
	Optional []string `yaml:"optional"`
}

type profileState struct {
	Dir string `yaml:"dir"`
}

func writeProfile(args []string, timeout time.Duration) (string, error) {
	action := "save_as"
	if len(args) > 0 {
		action = args[0]
	}
	fields := mapFields(args[1:])
	name := fields["name"]
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if err := validateProfileName(name); err != nil {
		return "", err
	}

	profile, err := profileForWrite(action, fields)
	if err != nil {
		return "", err
	}
	profile.Name = name
	if profile.State.Dir == "" || strings.HasPrefix(profile.State.Dir, "state/") {
		profile.State.Dir = defaultString(fields["stateDir"], "state/"+name)
	}

	dir, err := profileWriteDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create profile dir: %w", err)
	}
	path := filepath.Join(dir, name+".yaml")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("PROFILE_NAME_CONFLICT: profile already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check profile path: %w", err)
	}

	data, err := yaml.Marshal(profile)
	if err != nil {
		return "", fmt.Errorf("marshal profile: %w", err)
	}
	tmp := filepath.Join(dir, "."+name+".yaml")
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return "", fmt.Errorf("write profile temp: %w", err)
	}
	validation, validationIsErr, err := runILab([]string{"profile", "validate", tmp}, timeout)
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if validationIsErr {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("PROFILE_INVALID: profile validation failed: %s", strings.TrimSpace(validation))
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("save profile: %w", err)
	}
	validation, validationIsErr, err = runILab([]string{"profile", "validate", path}, timeout)
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if validationIsErr {
		_ = os.Remove(path)
		return "", fmt.Errorf("PROFILE_INVALID: profile validation failed after save: %s", strings.TrimSpace(validation))
	}

	operationID := operationID("profile_" + action)
	auditPath, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: operationID,
		Tool:        "profile_" + action,
		Actor:       "agent",
		Risk:        "LOW",
		Target: map[string]string{
			"profile": name,
			"path":    path,
		},
		Result: "ok",
	})
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}

	env := profileWriteEnvelope{
		OK:              true,
		Command:         "profile." + action,
		ContractVersion: supportedContractVersion,
		Data: profileWriteData{
			OperationID: operationID,
			Profile: profileWriteRef{
				Name:   name,
				Source: "user",
				Path:   path,
			},
			Validation: json.RawMessage(validation),
			AuditPath:  auditPath,
		},
		Warnings: []any{},
		Errors:   []any{},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func profileForWrite(action string, fields map[string]string) (profileYAML, error) {
	if action == "clone" {
		source := fields["source"]
		if source == "" {
			return profileYAML{}, fmt.Errorf("source is required")
		}
		path, err := resolveProfileSource(source)
		if err != nil {
			return profileYAML{}, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return profileYAML{}, fmt.Errorf("read source profile: %w", err)
		}
		var profile profileYAML
		if err := yaml.Unmarshal(data, &profile); err != nil {
			return profileYAML{}, fmt.Errorf("parse source profile: %w", err)
		}
		applyProfileOverrides(&profile, fields)
		return profile, nil
	}

	profile := profileYAML{
		Backend: defaultString(fields["backend"], "multipass"),
		VM: profileVM{
			OSImage: defaultString(fields["osImage"], "ubuntu-24.04"),
			Masters: intDefault(fields["masters"], 1),
			Workers: intDefault(fields["workers"], 2),
			Master:  profileNode{CPU: 2, Memory: "4G", Disk: "40G"},
			Worker:  profileNode{CPU: 2, Memory: "4G", Disk: "50G"},
		},
		Kubernetes: profileK8s{Version: "1.36", CNI: defaultString(fields["cni"], "flannel")},
		Addons:     profileAddons{Base: []string{"metrics-server"}, Optional: []string{}},
	}
	return profile, nil
}

func applyProfileOverrides(profile *profileYAML, fields map[string]string) {
	if fields["backend"] != "" {
		profile.Backend = fields["backend"]
	}
	if fields["cni"] != "" {
		profile.Kubernetes.CNI = fields["cni"]
	}
	if fields["osImage"] != "" {
		profile.VM.OSImage = fields["osImage"]
	}
	if fields["masters"] != "" {
		profile.VM.Masters = intDefault(fields["masters"], profile.VM.Masters)
	}
	if fields["workers"] != "" {
		profile.VM.Workers = intDefault(fields["workers"], profile.VM.Workers)
	}
}

func resolveProfileSource(source string) (string, error) {
	if filepath.IsAbs(source) || strings.ContainsRune(source, filepath.Separator) {
		if _, err := os.Stat(source); err != nil {
			return "", fmt.Errorf("PROFILE_NOT_FOUND: %s", source)
		}
		return source, nil
	}
	dir, err := profileWriteDir()
	if err == nil {
		path := filepath.Join(dir, source+".yaml")
		if _, statErr := os.Stat(path); statErr == nil {
			return path, nil
		}
	}
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	for _, path := range []string{
		filepath.Join(root, "envs", source+".yaml"),
		filepath.Join(root, "envs", source+".yaml.example"),
	} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("PROFILE_NOT_FOUND: %s", source)
}

func profileWriteDir() (string, error) {
	if dir := os.Getenv("INFRA_LAB_PROFILE_DIR"); dir != "" {
		return dir, nil
	}
	configHome := infraLabConfigHome()
	if configHome == "" {
		return "", fmt.Errorf("config home not found")
	}
	return filepath.Join(configHome, "profiles"), nil
}

func infraLabConfigHome() string {
	if dir := os.Getenv("INFRA_LAB_CONFIG_HOME"); dir != "" {
		return dir
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "infra-lab")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "infra-lab")
	}
	return ""
}

type profileAuditRecord struct {
	Time        string            `json:"time"`
	OperationID string            `json:"operationId"`
	Tool        string            `json:"tool"`
	Actor       string            `json:"actor"`
	Risk        string            `json:"risk"`
	Target      map[string]string `json:"target"`
	Result      string            `json:"result"`
}

func appendAudit(record profileAuditRecord) (string, error) {
	path, err := auditPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create audit dir: %w", err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	return path, nil
}

func auditPath() (string, error) {
	if path := os.Getenv("INFRA_LAB_AUDIT_PATH"); path != "" {
		return path, nil
	}
	if root, err := infraLabRoot(); err == nil {
		return filepath.Join(root, "state", ".audit", "operations.jsonl"), nil
	}
	configHome := infraLabConfigHome()
	if configHome == "" {
		return "", fmt.Errorf("audit path not found")
	}
	return filepath.Join(configHome, "audit", "operations.jsonl"), nil
}

func mapFields(args []string) map[string]string {
	fields := map[string]string{}
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		if ok {
			fields[key] = value
		}
	}
	return fields
}

func validateProfileName(name string) error {
	if strings.ContainsAny(name, `/\`) || strings.TrimSpace(name) != name || name == "" {
		return fmt.Errorf("invalid profile name: %q", name)
	}
	return nil
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func intDefault(value string, fallback int) int {
	var out int
	if _, err := fmt.Sscanf(value, "%d", &out); err == nil && out > 0 {
		return out
	}
	return fallback
}

func operationID(suffix string) string {
	return "op_" + time.Now().UTC().Format("20060102_150405") + "_" + suffix
}
