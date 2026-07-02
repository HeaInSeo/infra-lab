package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type setupEnvelope struct {
	OK              bool             `json:"ok"`
	Command         string           `json:"command"`
	ContractVersion string           `json:"contractVersion"`
	Data            setupCheckData   `json:"data"`
	Warnings        []setupFinding   `json:"warnings"`
	Errors          []setupErrorInfo `json:"errors"`
}

type setupCheckData struct {
	Ready        bool                   `json:"ready"`
	Root         string                 `json:"root"`
	Server       setupServerInfo        `json:"server"`
	Binaries     []setupBinaryStatus    `json:"binaries"`
	Bootstrap    setupBootstrapStatus   `json:"bootstrap"`
	Capabilities []string               `json:"capabilities"`
	Tools        toolCatalog            `json:"tools"`
	Clients      map[string]setupClient `json:"clients"`
	Findings     []setupFinding         `json:"findings"`
	NextSteps    []string               `json:"nextSteps"`
}

type setupServerInfo struct {
	Executable string `json:"executable"`
	Transport  string `json:"transport"`
}

type setupBinaryStatus struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Exists     bool   `json:"exists"`
	Executable bool   `json:"executable"`
}

type setupBootstrapStatus struct {
	InfraLabVersion string `json:"infraLabVersion"`
	ContractVersion string `json:"contractVersion"`
}

type setupClient struct {
	Status  string `json:"status"`
	Command string `json:"command,omitempty"`
	Config  string `json:"config,omitempty"`
	Note    string `json:"note,omitempty"`
}

type setupFinding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type setupErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func addSetupCheckTool(handlers map[string]toolHandler, info bootstrapInfo) {
	handlers["infra_lab.setup_check"] = toolHandler{
		tool: tool{
			Name:        "infra_lab.setup_check",
			Description: "Check whether infra-lab MCP is ready and show client registration guidance. Run this first after connecting.",
			InputSchema: emptySchema(),
		},
		call: func(_ json.RawMessage) (toolResult, error) {
			raw, err := setupCheckJSON(info, handlers)
			if err != nil {
				return toolResult{}, err
			}
			return toolResult{Content: []toolContent{{Type: "text", Text: raw}}}, nil
		},
	}
}

// setupEnv bootstraps infra-lab and runs the setup check exactly once,
// returning the parsed envelope for callers to format or act on.
func setupEnv() (setupEnvelope, error) {
	info, err := bootstrap()
	if err != nil {
		return setupEnvelope{}, err
	}
	raw, err := setupCheckJSON(info, nil)
	if err != nil {
		return setupEnvelope{}, err
	}
	var env setupEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return setupEnvelope{}, err
	}
	return env, nil
}

func SetupCheckText() (string, error) {
	env, err := setupEnv()
	if err != nil {
		return "", err
	}
	return formatSetupCheckText(env), nil
}

func formatSetupCheckText(env setupEnvelope) string {
	var b strings.Builder
	fmt.Fprintf(&b, "infra-lab MCP setup check\n")
	fmt.Fprintf(&b, "ready: %t\n", env.Data.Ready)
	fmt.Fprintf(&b, "root: %s\n", env.Data.Root)
	fmt.Fprintf(&b, "server: %s --transport %s\n", env.Data.Server.Executable, env.Data.Server.Transport)
	fmt.Fprintf(&b, "version: %s\n", env.Data.Bootstrap.InfraLabVersion)
	fmt.Fprintf(&b, "contract: %s\n", env.Data.Bootstrap.ContractVersion)
	fmt.Fprintf(&b, "capabilities: %d\n", len(env.Data.Capabilities))
	fmt.Fprintf(&b, "tools: %d total\n", env.Data.Tools.Summary.Total)
	fmt.Fprintf(&b, "  readOnly=%d evidence=%d planOnly=%d profileWrite=%d approvedMutation=%d destructive=%d operation=%d\n",
		env.Data.Tools.Summary.ReadOnly,
		env.Data.Tools.Summary.Evidence,
		env.Data.Tools.Summary.PlanOnly,
		env.Data.Tools.Summary.ProfileWrite,
		env.Data.Tools.Summary.ApprovedMutation,
		env.Data.Tools.Summary.Destructive,
		env.Data.Tools.Summary.Operation,
	)
	fmt.Fprintf(&b, "\nbinaries:\n")
	for _, bin := range env.Data.Binaries {
		fmt.Fprintf(&b, "  - %s: exists=%t executable=%t path=%s\n", bin.Name, bin.Exists, bin.Executable, bin.Path)
	}
	if len(env.Data.Findings) > 0 {
		fmt.Fprintf(&b, "\nfindings:\n")
		for _, finding := range env.Data.Findings {
			fmt.Fprintf(&b, "  - %s: %s\n", finding.Code, finding.Message)
		}
	}
	fmt.Fprintf(&b, "\nnext steps:\n")
	for _, step := range env.Data.NextSteps {
		fmt.Fprintf(&b, "  - %s\n", step)
	}
	return b.String()
}

func ClientConfigText(client string) (string, error) {
	env, err := setupEnv()
	if err != nil {
		return "", err
	}
	return clientConfigTextFromEnv(env, client)
}

func clientConfigTextFromEnv(env setupEnvelope, client string) (string, error) {
	switch strings.ToLower(client) {
	case "codex":
		return env.Data.Clients["codex"].Command + "\n", nil
	case "claude", "claude-desktop":
		return env.Data.Clients["claude"].Config + "\n", nil
	default:
		return "", fmt.Errorf("unsupported client %q; use codex or claude", client)
	}
}

func RunSetupMenu(r io.Reader, w io.Writer) error {
	reader := bufio.NewReader(r)

	// Bootstrap and run the setup check at most once per menu session
	// (not once per selection) and reuse the result across choices.
	var cachedEnv *setupEnvelope
	loadEnv := func() (setupEnvelope, error) {
		if cachedEnv != nil {
			return *cachedEnv, nil
		}
		env, err := setupEnv()
		if err != nil {
			return setupEnvelope{}, err
		}
		cachedEnv = &env
		return env, nil
	}

	for {
		fmt.Fprintln(w, "infra-lab MCP setup")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "1. 상태 점검")
		fmt.Fprintln(w, "2. Codex에 MCP 등록")
		fmt.Fprintln(w, "3. Claude Code에 MCP 등록")
		fmt.Fprintln(w, "4. Claude 설정 JSON 보기")
		fmt.Fprintln(w, "5. 종료")
		fmt.Fprint(w, "\n선택: ")

		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		choice := strings.TrimSpace(line)
		fmt.Fprintln(w, "")

		switch choice {
		case "1":
			env, err := loadEnv()
			if err != nil {
				fmt.Fprintf(w, "상태 점검 실패: %v\n\n", err)
			} else {
				fmt.Fprintln(w, formatSetupCheckText(env))
			}
		case "2":
			env, err := loadEnv()
			if err != nil {
				fmt.Fprintf(w, "Codex 등록 실패: %v\n\n", err)
				break
			}
			out, err := installCodexMCP(env)
			if err != nil {
				fmt.Fprintf(w, "Codex 등록 실패: %v\n\n", err)
			} else {
				fmt.Fprintln(w, out)
			}
		case "3":
			env, err := loadEnv()
			if err != nil {
				fmt.Fprintf(w, "Claude Code 등록 실패: %v\n\n", err)
				break
			}
			out, err := installClaudeMCP(env)
			if err != nil {
				fmt.Fprintf(w, "Claude Code 등록 실패: %v\n\n", err)
			} else {
				fmt.Fprintln(w, out)
			}
		case "4":
			env, err := loadEnv()
			if err != nil {
				fmt.Fprintf(w, "Claude 설정 생성 실패: %v\n\n", err)
				break
			}
			out, err := clientConfigTextFromEnv(env, "claude")
			if err != nil {
				fmt.Fprintf(w, "Claude 설정 생성 실패: %v\n\n", err)
			} else {
				fmt.Fprintln(w, out)
			}
		case "5", "q", "quit", "exit":
			fmt.Fprintln(w, "종료합니다.")
			return nil
		default:
			fmt.Fprintln(w, "1, 2, 3, 4, 5 중에서 선택하세요.")
		}

		if err == io.EOF {
			return nil
		}
	}
}

func InstallCodexMCP() (string, error) {
	env, err := setupEnv()
	if err != nil {
		return "", err
	}
	return installCodexMCP(env)
}

// installCodexMCP always removes any existing 'infra-lab' registration
// before re-adding it, so the registered command/env stays in sync with
// the current binary path and INFRA_LAB_ROOT even after a rebuild or move.
// The previous `mcp get` short-circuit reported "already registered"
// without ever refreshing a stale path (infra-lab#21).
func installCodexMCP(env setupEnvelope) (string, error) {
	if !env.Data.Ready {
		return "", fmt.Errorf("setup check is not ready")
	}

	if _, err := exec.LookPath("codex"); err != nil {
		return "", fmt.Errorf("codex CLI not found in PATH")
	}
	_ = exec.Command("codex", "mcp", "remove", "infra-lab").Run()

	cmd := exec.Command(
		"codex", "mcp", "add", "infra-lab",
		"--env", "INFRA_LAB_ROOT="+env.Data.Root,
		"--",
		env.Data.Server.Executable,
		"--transport", "stdio",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return "Codex MCP 서버 'infra-lab' 등록이 완료되었습니다 (기존 등록이 있었다면 최신 경로로 갱신됨). Codex를 재시작하거나 새 세션을 여세요.", nil
}

func InstallClaudeMCP() (string, error) {
	env, err := setupEnv()
	if err != nil {
		return "", err
	}
	return installClaudeMCP(env)
}

// installClaudeMCP mirrors installCodexMCP: always remove then re-add so
// the registration never goes stale. Unlike codex, `claude mcp add` errors
// on a duplicate name instead of overwriting, so the explicit remove step
// is required here, not just a staleness nicety (infra-lab#21).
func installClaudeMCP(env setupEnvelope) (string, error) {
	if !env.Data.Ready {
		return "", fmt.Errorf("setup check is not ready")
	}

	if _, err := exec.LookPath("claude"); err != nil {
		return "", fmt.Errorf("claude CLI not found in PATH")
	}
	_ = exec.Command("claude", "mcp", "remove", "infra-lab", "--scope", "user").Run()

	cmd := exec.Command(
		"claude", "mcp", "add", "infra-lab",
		"--scope", "user",
		"--env", "INFRA_LAB_ROOT="+env.Data.Root,
		"--",
		env.Data.Server.Executable,
		"--transport", "stdio",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return "Claude Code MCP 서버 'infra-lab' 등록이 완료되었습니다 (scope: user, 기존 등록이 있었다면 최신 경로로 갱신됨). Claude Code를 재시작하거나 새 세션을 여세요.", nil
}

func setupCheckJSON(info bootstrapInfo, handlers map[string]toolHandler) (string, error) {
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		exe = filepath.Join(root, "bin", "infra-lab-mcp")
	}
	exe, _ = filepath.Abs(exe)

	capabilities := make([]string, 0, len(info.Capabilities))
	for capability := range info.Capabilities {
		capabilities = append(capabilities, capability)
	}
	sort.Strings(capabilities)

	ilabPath := filepath.Join(root, "bin", "ilab")
	mcpPath := exe
	binaries := []setupBinaryStatus{
		binaryStatus("ilab", ilabPath),
		binaryStatus("infra-lab-mcp", mcpPath),
	}

	findings := []setupFinding{}
	for _, bin := range binaries {
		if !bin.Exists {
			findings = append(findings, setupFinding{
				Code:    "BINARY_NOT_FOUND",
				Message: fmt.Sprintf("%s binary was not found", bin.Name),
				Field:   bin.Path,
			})
			continue
		}
		if !bin.Executable {
			findings = append(findings, setupFinding{
				Code:    "BINARY_NOT_EXECUTABLE",
				Message: fmt.Sprintf("%s binary is not executable", bin.Name),
				Field:   bin.Path,
			})
		}
	}

	ready := len(findings) == 0
	clients := map[string]setupClient{
		"codex": {
			Status:  "manual_registration_required",
			Command: codexAddCommand(root, exe),
			Note:    "Run this once outside the MCP server process. Restart Codex after registration.",
		},
		"claude": {
			Status: "manual_registration_required",
			Config: claudeConfig(root, exe),
			Note:   "Add this to the Claude Desktop MCP server configuration, then restart Claude.",
		},
	}

	env := setupEnvelope{
		OK:              ready,
		Command:         "mcp.setup_check",
		ContractVersion: supportedContractVersion,
		Data: setupCheckData{
			Ready: ready,
			Root:  root,
			Server: setupServerInfo{
				Executable: exe,
				Transport:  "stdio",
			},
			Binaries: binaries,
			Bootstrap: setupBootstrapStatus{
				InfraLabVersion: info.InfraLabVersion,
				ContractVersion: info.ContractVersion,
			},
			Capabilities: capabilities,
			Tools:        toolCatalogFromHandlers(handlersForCatalog(info, handlers)),
			Clients:      clients,
			Findings:     findings,
			NextSteps: []string{
				"Ask the agent to run infra_lab.setup_check first after MCP connection.",
				"Use infra_lab.doctor for host prerequisite diagnostics.",
				"Use infra_lab.collect_snapshot before diagnosing an existing lab.",
			},
		},
		Warnings: []setupFinding{},
		Errors:   []setupErrorInfo{},
	}
	if !ready {
		env.Errors = []setupErrorInfo{{Code: "MCP_SETUP_NOT_READY", Message: "infra-lab MCP setup check found blocking issues"}}
	}

	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func binaryStatus(name, path string) setupBinaryStatus {
	st, err := os.Stat(path)
	if err != nil {
		return setupBinaryStatus{Name: name, Path: path, Exists: false, Executable: false}
	}
	return setupBinaryStatus{
		Name:       name,
		Path:       path,
		Exists:     true,
		Executable: !st.IsDir() && st.Mode().Perm()&0111 != 0,
	}
}

func codexAddCommand(root, exe string) string {
	return fmt.Sprintf("codex mcp add infra-lab --env INFRA_LAB_ROOT=%s -- %s --transport stdio", shellQuote(root), shellQuote(exe))
}

func claudeConfig(root, exe string) string {
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"infra-lab": map[string]any{
				"command": exe,
				"args":    []string{"--transport", "stdio"},
				"env": map[string]string{
					"INFRA_LAB_ROOT": root,
				},
			},
		},
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(out)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '-' || r == '_' || r == '=' || r == ':' ||
			(r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
