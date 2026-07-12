package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type operationEnvelope struct {
	OK              bool          `json:"ok"`
	Command         string        `json:"command"`
	ContractVersion string        `json:"contractVersion"`
	Data            any           `json:"data"`
	Warnings        []any         `json:"warnings"`
	Errors          []interface{} `json:"errors"`
}

type operationRecord struct {
	OperationID string            `json:"operationId"`
	Tool        string            `json:"tool"`
	Status      string            `json:"status"`
	Risk        string            `json:"risk"`
	Destructive bool              `json:"destructive"`
	Target      operationTarget   `json:"target"`
	CreatedAt   string            `json:"createdAt"`
	ExpiresAt   string            `json:"expiresAt"`
	StartedAt   string            `json:"startedAt,omitempty"`
	FinishedAt  string            `json:"finishedAt,omitempty"`
	Approval    operationApproval `json:"approval"`
	Steps       []operationStep   `json:"steps"`
	ErrorCode   string            `json:"errorCode,omitempty"`
	Error       string            `json:"error,omitempty"`
}

type operationTarget struct {
	Env               string `json:"env"`
	VM                string `json:"vm,omitempty"`
	Name              string `json:"name,omitempty"`
	ContextDir        string `json:"contextDir,omitempty"`
	Dockerfile        string `json:"dockerfile,omitempty"`
	Image             string `json:"image,omitempty"`
	Builder           string `json:"builder,omitempty"`
	SourceDigest      string `json:"sourceDigest,omitempty"`
	Addon             string `json:"addon,omitempty"`
	Profile           string `json:"profile,omitempty"`
	ProfileDigest     string `json:"profileDigest,omitempty"`
	TargetFingerprint string `json:"targetFingerprint"`
}

type operationApproval struct {
	Required  bool   `json:"required"`
	Status    string `json:"status"`
	Mode      string `json:"mode"`
	TokenHint string `json:"tokenHint,omitempty"`
}

type operationStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type operationPrepareData struct {
	OperationID       string              `json:"operationId"`
	ApprovalToken     string              `json:"approvalToken"`
	ExpiresAt         string              `json:"expiresAt"`
	PlanFingerprint   string              `json:"planFingerprint"`
	TargetFingerprint string              `json:"targetFingerprint"`
	Approval          operationApproval   `json:"approval"`
	Risk              string              `json:"risk"`
	Target            operationTarget     `json:"target"`
	Steps             []string            `json:"steps"`
	Operation         operationRecord     `json:"operation"`
	OperationPath     string              `json:"operationPath"`
	Warnings          []map[string]string `json:"findings,omitempty"`
}

type operationLogsData struct {
	OperationID string `json:"operationId"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
}

type operationLockRecord struct {
	OperationID string `json:"operationId"`
	Env         string `json:"env"`
	Tool        string `json:"tool"`
	StartedAt   string `json:"startedAt"`
	ExpiresAt   string `json:"expiresAt"`
	PID         int    `json:"pid,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	Stale       bool   `json:"stale"`
	Path        string `json:"path"`
}

type operationLocksData struct {
	Locks []operationLockRecord `json:"locks"`
}

type profileValidationEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		Profile struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"profile"`
		Normalized struct {
			StateDir string `json:"stateDir"`
		} `json:"normalized"`
	} `json:"data"`
}

type vmListEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		VMs []struct {
			Name    string `json:"name"`
			Managed bool   `json:"managed"`
			Env     string `json:"env"`
			Backend string `json:"backend"`
			State   string `json:"state"`
			IPv4    string `json:"ipv4"`
		} `json:"vms"`
	} `json:"data"`
}

func handleOperation(args []string, timeout time.Duration) (string, error) {
	action := ""
	if len(args) > 0 {
		action = args[0]
	}
	fields := mapFields(args[1:])
	switch action {
	case "addon_install_prepare":
		return prepareAddonInstall(fields)
	case "addon_install_commit":
		return commitAddonInstall(fields, timeout)
	case "env_up_prepare":
		return prepareEnvUp(fields)
	case "env_up_commit":
		return commitEnvUp(fields, timeout)
	case "env_down_prepare", "env_clean_prepare", "env_rebuild_prepare", "addon_uninstall_prepare":
		return prepareDestructive(action, fields)
	case "env_down_commit", "env_clean_commit", "env_rebuild_commit", "addon_uninstall_commit":
		return commitDestructive(action, fields, timeout)
	case "libvirt_vm_resume_prepare":
		return prepareLibvirtVMResume(fields)
	case "libvirt_vm_resume_commit":
		return commitLibvirtVMResume(fields, timeout)
	case "container_image_build_push_prepare":
		return prepareContainerImageBuildPush(fields)
	case "container_image_build_push_commit":
		return commitContainerImageBuildPush(fields, timeout)
	case "status":
		return operationStatus(fields["operationId"])
	case "logs":
		return operationLogs(fields["operationId"])
	case "approve":
		return operationApprove(fields["operationId"])
	case "cancel":
		return operationCancel(fields["operationId"])
	case "locks":
		return operationLocks()
	case "unlock_stale":
		return operationUnlockStale(fields["env"])
	default:
		return "", fmt.Errorf("unsupported operation action: %s", action)
	}
}

func prepareLibvirtVMResume(fields map[string]string) (string, error) {
	env := fields["env"]
	vm := fields["vm"]
	if env == "" || vm == "" {
		return "", fmt.Errorf("env and vm are required")
	}
	if err := validateTokenPart(env); err != nil {
		return "", err
	}
	if err := validateTokenPart(vm); err != nil {
		return "", err
	}
	if err := validateLibvirtVMTarget(env, vm); err != nil {
		return "", err
	}

	now := time.Now().UTC()
	op := operationRecord{
		OperationID: operationID("libvirt_vm_resume"),
		Tool:        "libvirt_vm_resume",
		Status:      "PREPARED",
		Risk:        "HIGH",
		Destructive: false,
		Target: operationTarget{
			Env:               env,
			VM:                vm,
			TargetFingerprint: digest("libvirt_vm_resume", env, vm),
		},
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(1 * time.Hour).Format(time.RFC3339),
		Approval: operationApproval{
			Required: true,
			Status:   "required",
			Mode:     "token-v1",
		},
		Steps: []operationStep{
			{Name: "collect pre-snapshot", Status: "pending"},
			{Name: "run libvirt vm resume", Status: "pending"},
			{Name: "collect post-snapshot", Status: "pending"},
		},
	}
	token, err := approvalToken(op)
	if err != nil {
		return "", err
	}
	op.Approval.TokenHint = token[:min(18, len(token))]
	path, err := writeOperation(op)
	if err != nil {
		return "", err
	}
	data := operationPrepareData{
		OperationID:       op.OperationID,
		ApprovalToken:     token,
		ExpiresAt:         op.ExpiresAt,
		PlanFingerprint:   digest("plan", "libvirt_vm_resume", env, vm),
		TargetFingerprint: op.Target.TargetFingerprint,
		Approval:          op.Approval,
		Risk:              op.Risk,
		Target:            op.Target,
		Steps:             []string{"collect pre-snapshot", "run libvirt vm resume", "collect post-snapshot"},
		Operation:         op,
		OperationPath:     path,
		Warnings: []map[string]string{{
			"code":    "VERIFY_STORAGE_BEFORE_RESUME",
			"message": "resume only after host storage pressure or block I/O cause has been addressed",
		}},
	}
	return encodeOperationEnvelope("libvirt.vm.resume.prepare", data)
}

func commitLibvirtVMResume(fields map[string]string, timeout time.Duration) (string, error) {
	op, err := verifyPreparedOperation(fields, "libvirt_vm_resume")
	if err != nil {
		return "", err
	}
	if err := validateLibvirtVMTarget(op.Target.Env, op.Target.VM); err != nil {
		return "", err
	}
	release, err := acquireEnvLock(op)
	if err != nil {
		return "", err
	}
	defer release()
	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "libvirt_vm_resume_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "started",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}

	op.Status = "RUNNING"
	op.StartedAt = time.Now().UTC().Format(time.RFC3339)
	op.Approval.Status = "approved"
	markStep(&op, "collect pre-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "pre-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect pre-snapshot")
	}
	markStep(&op, "collect pre-snapshot", "succeeded")
	markStep(&op, "run libvirt vm resume", "running")
	_, _ = writeOperation(op)
	if err := runLoggedCommand(op, timeout, "virsh", "-c", "qemu:///system", "resume", op.Target.VM); err != nil {
		return failOperation(op, "COMMAND_FAILED", err, "run libvirt vm resume")
	}
	markStep(&op, "run libvirt vm resume", "succeeded")
	markStep(&op, "collect post-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "post-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect post-snapshot")
	}
	op.Status = "SUCCEEDED"
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	markStep(&op, "collect post-snapshot", "succeeded")
	_, _ = writeOperation(op)
	_, _ = appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "libvirt_vm_resume_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "ok",
	})
	return encodeOperationEnvelope("libvirt.vm.resume.commit", op)
}

func validateLibvirtVMTarget(env, vm string) error {
	raw, isErr, err := runILab([]string{"vm", "list"}, 30*time.Second)
	if err != nil {
		return err
	}
	if isErr {
		return fmt.Errorf("VM_LIST_FAILED: %s", strings.TrimSpace(raw))
	}
	return validateLibvirtVMTargetFromList(raw, env, vm)
}

func validateLibvirtVMTargetFromList(raw, env, vm string) error {
	var parsed vmListEnvelope
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return err
	}
	for _, candidate := range parsed.Data.VMs {
		if candidate.Name == vm && candidate.Env == env && candidate.Backend == "libvirt" && candidate.Managed {
			return nil
		}
	}
	return fmt.Errorf("VM_NOT_MANAGED_BY_ENV: vm %q is not a managed libvirt VM in env %q", vm, env)
}

func prepareContainerImageBuildPush(fields map[string]string) (string, error) {
	target, err := resolveContainerImageBuildTarget(fields)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	op := operationRecord{
		OperationID: operationID("container_image_build_push"),
		Tool:        "container_image_build_push",
		Status:      "PREPARED",
		Risk:        "HIGH",
		Destructive: false,
		Target:      target,
		CreatedAt:   now.Format(time.RFC3339),
		ExpiresAt:   now.Add(1 * time.Hour).Format(time.RFC3339),
		Approval: operationApproval{
			Required: true,
			Status:   "required",
			Mode:     "token-v1",
		},
		Steps: []operationStep{
			{Name: "validate image target", Status: "pending"},
			{Name: "build container image", Status: "pending"},
			{Name: "push container image", Status: "pending"},
		},
	}
	token, err := approvalToken(op)
	if err != nil {
		return "", err
	}
	op.Approval.TokenHint = token[:min(18, len(token))]
	path, err := writeOperation(op)
	if err != nil {
		return "", err
	}
	data := operationPrepareData{
		OperationID:       op.OperationID,
		ApprovalToken:     token,
		ExpiresAt:         op.ExpiresAt,
		PlanFingerprint:   digest("plan", "container_image_build_push", target.Name, target.Image, target.SourceDigest),
		TargetFingerprint: target.TargetFingerprint,
		Approval:          op.Approval,
		Risk:              op.Risk,
		Target:            op.Target,
		Steps:             []string{"validate image target", "build container image", "push container image"},
		Operation:         op,
		OperationPath:     path,
		Warnings: []map[string]string{{
			"code":    "REGISTRY_PUSH_MUTATES_REMOTE_STATE",
			"message": "commit builds from the approved source fingerprint and pushes the image tag to the registry",
		}},
	}
	return encodeOperationEnvelope("container.image.build_push.prepare", data)
}

func commitContainerImageBuildPush(fields map[string]string, timeout time.Duration) (string, error) {
	op, err := verifyPreparedOperation(fields, "container_image_build_push")
	if err != nil {
		return "", err
	}
	target, err := resolveContainerImageBuildTarget(map[string]string{
		"name":       op.Target.Name,
		"contextDir": op.Target.ContextDir,
		"dockerfile": op.Target.Dockerfile,
		"image":      op.Target.Image,
		"builder":    op.Target.Builder,
	})
	if err != nil {
		return "", err
	}
	if target.SourceDigest != op.Target.SourceDigest {
		return "", fmt.Errorf("SOURCE_CHANGED: prepared source digest %s, current source digest %s", op.Target.SourceDigest, target.SourceDigest)
	}
	release, err := acquireNamedLock("image-"+target.Name, op)
	if err != nil {
		return "", err
	}
	defer release()
	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "container_image_build_push_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "started",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}

	op.Status = "RUNNING"
	op.StartedAt = time.Now().UTC().Format(time.RFC3339)
	op.Approval.Status = "approved"
	markStep(&op, "validate image target", "succeeded")
	markStep(&op, "build container image", "running")
	_, _ = writeOperation(op)
	dockerfileRel, err := filepath.Rel(op.Target.ContextDir, op.Target.Dockerfile)
	if err != nil {
		return failOperation(op, "COMMAND_FAILED", err, "build container image")
	}
	if err := runLoggedCommand(op, timeout, op.Target.Builder, "build", "-f", dockerfileRel, "-t", op.Target.Image, op.Target.ContextDir); err != nil {
		return failOperation(op, "COMMAND_FAILED", err, "build container image")
	}
	markStep(&op, "build container image", "succeeded")
	markStep(&op, "push container image", "running")
	_, _ = writeOperation(op)
	if err := runLoggedCommand(op, timeout, op.Target.Builder, "push", op.Target.Image); err != nil {
		return failOperation(op, "COMMAND_FAILED", err, "push container image")
	}
	markStep(&op, "push container image", "succeeded")
	op.Status = "SUCCEEDED"
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	_, _ = writeOperation(op)
	_, _ = appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "container_image_build_push_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "ok",
	})
	return encodeOperationEnvelope("container.image.build_push.commit", op)
}

func resolveContainerImageBuildTarget(fields map[string]string) (operationTarget, error) {
	name := fields["name"]
	contextDir := fields["contextDir"]
	dockerfile := fields["dockerfile"]
	image := fields["image"]
	builder := fields["builder"]
	if name == "" || contextDir == "" || image == "" {
		return operationTarget{}, fmt.Errorf("name, contextDir, and image are required")
	}
	if err := validateTokenPart(name); err != nil {
		return operationTarget{}, err
	}
	if err := validateImageReference(image); err != nil {
		return operationTarget{}, err
	}
	resolvedBuilder, err := resolveImageBuilder(builder)
	if err != nil {
		return operationTarget{}, err
	}
	resolvedContext, err := resolveAllowedBuildContext(contextDir)
	if err != nil {
		return operationTarget{}, err
	}
	resolvedDockerfile, err := resolveBuildDockerfile(resolvedContext, dockerfile)
	if err != nil {
		return operationTarget{}, err
	}
	sourceDigest, err := containerImageSourceDigest(resolvedContext, resolvedDockerfile)
	if err != nil {
		return operationTarget{}, err
	}
	return operationTarget{
		Name:              name,
		ContextDir:        resolvedContext,
		Dockerfile:        resolvedDockerfile,
		Image:             image,
		Builder:           resolvedBuilder,
		SourceDigest:      sourceDigest,
		TargetFingerprint: digest("container_image_build_push", name, resolvedContext, resolvedDockerfile, image, resolvedBuilder, sourceDigest),
	}, nil
}

func resolveImageBuilder(builder string) (string, error) {
	switch builder {
	case "":
		for _, candidate := range []string{"podman", "docker"} {
			if _, err := exec.LookPath(candidate); err == nil {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("NO_CONTAINER_BUILDER: podman or docker is required")
	case "podman", "docker":
		if _, err := exec.LookPath(builder); err != nil {
			return "", fmt.Errorf("CONTAINER_BUILDER_NOT_FOUND: %s", builder)
		}
		return builder, nil
	default:
		return "", fmt.Errorf("unsupported builder: %s", builder)
	}
}

func validateImageReference(image string) error {
	if strings.TrimSpace(image) != image || image == "" {
		return fmt.Errorf("invalid image reference: %q", image)
	}
	if strings.Contains(image, "://") || strings.Contains(image, "@") {
		return fmt.Errorf("invalid image reference: %q", image)
	}
	for _, r := range image {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '.', '_', '-', '/', ':':
			continue
		default:
			return fmt.Errorf("invalid image reference: %q", image)
		}
	}
	firstSlash := strings.IndexByte(image, '/')
	if firstSlash <= 0 {
		return fmt.Errorf("IMAGE_REQUIRES_REGISTRY: %q", image)
	}
	registry := image[:firstSlash]
	if !strings.Contains(registry, ".") && !strings.Contains(registry, ":") && registry != "localhost" {
		return fmt.Errorf("IMAGE_REQUIRES_REGISTRY: %q", image)
	}
	tagIndex := strings.LastIndexByte(image, ':')
	if tagIndex <= firstSlash || tagIndex == len(image)-1 {
		return fmt.Errorf("IMAGE_REQUIRES_TAG: %q", image)
	}
	if strings.Contains(image[firstSlash+1:tagIndex], "//") {
		return fmt.Errorf("invalid image reference: %q", image)
	}
	return nil
}

func resolveAllowedBuildContext(contextDir string) (string, error) {
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(contextDir) {
		contextDir = filepath.Join(root, contextDir)
	}
	absContext, err := filepath.Abs(contextDir)
	if err != nil {
		return "", err
	}
	resolvedContext, err := filepath.EvalSymlinks(absContext)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolvedContext)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("CONTEXT_NOT_DIRECTORY: %s", resolvedContext)
	}
	roots, err := imageBuildAllowedRoots(root)
	if err != nil {
		return "", err
	}
	for _, allowed := range roots {
		if isPathWithin(allowed, resolvedContext) {
			return resolvedContext, nil
		}
	}
	return "", fmt.Errorf("CONTEXT_OUTSIDE_ALLOWED_ROOTS: %s", resolvedContext)
}

func imageBuildAllowedRoots(root string) ([]string, error) {
	raw := os.Getenv("INFRA_LAB_IMAGE_BUILD_ROOTS")
	candidates := []string{}
	if raw == "" {
		candidates = append(candidates, filepath.Dir(root))
	} else {
		candidates = filepath.SplitList(raw)
	}
	roots := []string{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			return nil, err
		}
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("ALLOWED_ROOT_NOT_DIRECTORY: %s", resolved)
		}
		roots = append(roots, resolved)
	}
	return roots, nil
}

func resolveBuildDockerfile(contextDir, dockerfile string) (string, error) {
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	if filepath.IsAbs(dockerfile) {
		return "", fmt.Errorf("dockerfile must be relative to contextDir")
	}
	if strings.ContainsAny(dockerfile, `\`) {
		return "", fmt.Errorf("invalid dockerfile path: %q", dockerfile)
	}
	path := filepath.Clean(filepath.Join(contextDir, dockerfile))
	if !isPathWithin(contextDir, path) {
		return "", fmt.Errorf("DOCKERFILE_OUTSIDE_CONTEXT: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("DOCKERFILE_IS_DIRECTORY: %s", path)
	}
	return path, nil
}

func isPathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != "" && rel != ".." && !strings.HasPrefix(rel, "../"))
}

func containerImageSourceDigest(contextDir, dockerfile string) (string, error) {
	gitRoot, gitErr := commandOutput("git", "-C", contextDir, "rev-parse", "--show-toplevel")
	if gitErr == nil {
		head, err := commandOutput("git", "-C", contextDir, "rev-parse", "HEAD")
		if err != nil {
			return "", err
		}
		status, err := commandOutput("git", "-C", contextDir, "status", "--porcelain=v1")
		if err != nil {
			return "", err
		}
		return digest("git", strings.TrimSpace(gitRoot), strings.TrimSpace(head), status), nil
	}
	info, err := os.Stat(dockerfile)
	if err != nil {
		return "", err
	}
	return digest("file", dockerfile, info.ModTime().UTC().Format(time.RFC3339Nano), fmt.Sprintf("%d", info.Size())), nil
}

func commandOutput(command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func prepareDestructive(action string, fields map[string]string) (string, error) {
	env := fields["env"]
	if env == "" {
		return "", fmt.Errorf("env is required")
	}
	if err := validateTokenPart(env); err != nil {
		return "", err
	}
	tool := strings.TrimSuffix(action, "_prepare")
	if tool == "env_rebuild" && fields["profile"] == "" {
		return "", fmt.Errorf("profile is required")
	}
	if tool == "addon_uninstall" && fields["addon"] == "" {
		return "", fmt.Errorf("addon is required")
	}
	if fields["profile"] != "" {
		validation, isErr, err := runILab([]string{"profile", "validate", fields["profile"]}, 30*time.Second)
		if err != nil {
			return "", err
		}
		if isErr {
			return "", fmt.Errorf("PROFILE_INVALID: %s", strings.TrimSpace(validation))
		}
	}

	profileDigest := ""
	if fields["profile"] != "" {
		profileDigest = digest("profile", fields["profile"])
	}
	now := time.Now().UTC()
	op := operationRecord{
		OperationID: operationID(tool),
		Tool:        tool,
		Status:      "PREPARED",
		Risk:        "HIGH",
		Destructive: true,
		Target: operationTarget{
			Env:               env,
			Addon:             fields["addon"],
			Profile:           fields["profile"],
			ProfileDigest:     profileDigest,
			TargetFingerprint: digest(tool, env, fields["addon"], fields["profile"]),
		},
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(1 * time.Hour).Format(time.RFC3339),
		Approval: operationApproval{
			Required: true,
			Status:   "required",
			Mode:     "token-v1",
		},
		Steps: destructiveSteps(tool),
	}
	token, err := approvalToken(op)
	if err != nil {
		return "", err
	}
	op.Approval.TokenHint = token[:min(18, len(token))]
	path, err := writeOperation(op)
	if err != nil {
		return "", err
	}
	stepNames := make([]string, 0, len(op.Steps))
	for _, step := range op.Steps {
		stepNames = append(stepNames, step.Name)
	}
	data := operationPrepareData{
		OperationID:       op.OperationID,
		ApprovalToken:     token,
		ExpiresAt:         op.ExpiresAt,
		PlanFingerprint:   digest("plan", tool, env, fields["addon"], fields["profile"]),
		TargetFingerprint: op.Target.TargetFingerprint,
		Approval:          op.Approval,
		Risk:              op.Risk,
		Target:            op.Target,
		Steps:             stepNames,
		Operation:         op,
		OperationPath:     path,
	}
	return encodeOperationEnvelope(commandForTool(tool)+".prepare", data)
}

func commitDestructive(action string, fields map[string]string, timeout time.Duration) (string, error) {
	tool := strings.TrimSuffix(action, "_commit")
	op, err := verifyPreparedOperation(fields, tool)
	if err != nil {
		return "", err
	}
	release, err := acquireEnvLock(op)
	if err != nil {
		return "", err
	}
	defer release()
	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        tool + "_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "started",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}

	op.Status = "RUNNING"
	op.StartedAt = time.Now().UTC().Format(time.RFC3339)
	op.Approval.Status = "approved"
	markStep(&op, "collect pre-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "pre-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect pre-snapshot")
	}
	markStep(&op, "collect pre-snapshot", "succeeded")
	runStep := destructiveRunStep(tool)
	markStep(&op, runStep, "running")
	_, _ = writeOperation(op)
	if err := runDestructiveCommand(op, timeout); err != nil {
		return failOperation(op, "COMMAND_FAILED", err, runStep)
	}
	markStep(&op, runStep, "succeeded")
	markStep(&op, "collect post-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "post-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect post-snapshot")
	}
	op.Status = "SUCCEEDED"
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	markStep(&op, "collect post-snapshot", "succeeded")
	_, _ = writeOperation(op)
	_, _ = appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        tool + "_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "ok",
	})
	return encodeOperationEnvelope(commandForTool(tool)+".commit", op)
}

func prepareEnvUp(fields map[string]string) (string, error) {
	profile := fields["profile"]
	if profile == "" {
		return "", fmt.Errorf("profile is required")
	}
	validation, isErr, err := runILab([]string{"profile", "validate", profile}, 30*time.Second)
	if err != nil {
		return "", err
	}
	if isErr {
		return "", fmt.Errorf("PROFILE_INVALID: %s", strings.TrimSpace(validation))
	}
	var parsed profileValidationEnvelope
	if err := json.Unmarshal([]byte(validation), &parsed); err != nil {
		return "", err
	}
	env := fields["env"]
	if env == "" {
		env = filepath.Base(parsed.Data.Normalized.StateDir)
	}
	if env == "" || env == "." {
		env = parsed.Data.Profile.Name
	}
	if err := validateTokenPart(env); err != nil {
		return "", err
	}
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, "state", env)); err == nil {
		return "", fmt.Errorf("ENV_ALREADY_EXISTS: %s", env)
	} else if !os.IsNotExist(err) {
		return "", err
	}

	profileDigest := digest("profile", profile, validation)
	now := time.Now().UTC()
	op := operationRecord{
		OperationID: operationID("env_up"),
		Tool:        "env_up",
		Status:      "PREPARED",
		Risk:        "HIGH",
		Destructive: false,
		Target: operationTarget{
			Env:               env,
			Profile:           profile,
			ProfileDigest:     profileDigest,
			TargetFingerprint: digest("env_up", env, profile, profileDigest),
		},
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(1 * time.Hour).Format(time.RFC3339),
		Approval: operationApproval{
			Required: true,
			Status:   "required",
			Mode:     "token-v1",
		},
		Steps: []operationStep{
			{Name: "validate profile", Status: "succeeded"},
			{Name: "collect pre-snapshot", Status: "pending"},
			{Name: "run env up", Status: "pending"},
			{Name: "collect post-snapshot", Status: "pending"},
		},
	}
	token, err := approvalToken(op)
	if err != nil {
		return "", err
	}
	op.Approval.TokenHint = token[:min(18, len(token))]
	path, err := writeOperation(op)
	if err != nil {
		return "", err
	}
	data := operationPrepareData{
		OperationID:       op.OperationID,
		ApprovalToken:     token,
		ExpiresAt:         op.ExpiresAt,
		PlanFingerprint:   digest("plan", "env_up", env, profile, profileDigest),
		TargetFingerprint: op.Target.TargetFingerprint,
		Approval:          op.Approval,
		Risk:              op.Risk,
		Target:            op.Target,
		Steps:             []string{"validate profile", "collect pre-snapshot", "run env up", "collect post-snapshot"},
		Operation:         op,
		OperationPath:     path,
	}
	return encodeOperationEnvelope("env.up.prepare", data)
}

func commitEnvUp(fields map[string]string, timeout time.Duration) (string, error) {
	op, err := verifyPreparedOperation(fields, "env_up")
	if err != nil {
		return "", err
	}
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, "state", op.Target.Env)); err == nil {
		return "", fmt.Errorf("ENV_ALREADY_EXISTS: %s", op.Target.Env)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	release, err := acquireEnvLock(op)
	if err != nil {
		return "", err
	}
	defer release()
	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "env_up_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      map[string]string{"env": op.Target.Env, "profile": op.Target.Profile},
		Result:      "started",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}

	op.Status = "RUNNING"
	op.StartedAt = time.Now().UTC().Format(time.RFC3339)
	op.Approval.Status = "approved"
	markStep(&op, "collect pre-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "pre-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect pre-snapshot")
	}
	markStep(&op, "collect pre-snapshot", "succeeded")
	markStep(&op, "run env up", "running")
	_, _ = writeOperation(op)
	if err := runEnvUpCommand(op, timeout); err != nil {
		return failOperation(op, "COMMAND_FAILED", err, "run env up")
	}
	markStep(&op, "run env up", "succeeded")
	markStep(&op, "collect post-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "post-snapshot.json", op.Target.Env); err != nil {
		return failOperation(op, "SNAPSHOT_FAILED", err, "collect post-snapshot")
	}
	op.Status = "SUCCEEDED"
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	markStep(&op, "collect post-snapshot", "succeeded")
	_, _ = writeOperation(op)
	_, _ = appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "env_up_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      map[string]string{"env": op.Target.Env, "profile": op.Target.Profile},
		Result:      "ok",
	})
	return encodeOperationEnvelope("env.up.commit", op)
}

func prepareAddonInstall(fields map[string]string) (string, error) {
	env := fields["env"]
	addon := fields["addon"]
	if env == "" || addon == "" {
		return "", fmt.Errorf("env and addon are required")
	}
	if err := validateTokenPart(env); err != nil {
		return "", err
	}
	if err := validateTokenPart(addon); err != nil {
		return "", err
	}

	now := time.Now().UTC()
	op := operationRecord{
		OperationID: operationID("addon_install"),
		Tool:        "addon_install",
		Status:      "PREPARED",
		Risk:        "MEDIUM",
		Destructive: false,
		Target: operationTarget{
			Env:               env,
			Addon:             addon,
			TargetFingerprint: digest("addon_install", env, addon),
		},
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(1 * time.Hour).Format(time.RFC3339),
		Approval: operationApproval{
			Required: true,
			Status:   "required",
			Mode:     "token-v1",
		},
		Steps: []operationStep{
			{Name: "collect pre-snapshot", Status: "pending"},
			{Name: "run addon install", Status: "pending"},
			{Name: "run addon verify", Status: "pending"},
			{Name: "collect post-snapshot", Status: "pending"},
		},
	}
	token, err := approvalToken(op)
	if err != nil {
		return "", err
	}
	op.Approval.TokenHint = token[:min(18, len(token))]
	path, err := writeOperation(op)
	if err != nil {
		return "", err
	}
	data := operationPrepareData{
		OperationID:       op.OperationID,
		ApprovalToken:     token,
		ExpiresAt:         op.ExpiresAt,
		PlanFingerprint:   digest("plan", "addon_install", env, addon),
		TargetFingerprint: op.Target.TargetFingerprint,
		Approval:          op.Approval,
		Risk:              op.Risk,
		Target:            op.Target,
		Steps:             []string{"collect pre-snapshot", "run addon install", "run addon verify", "collect post-snapshot"},
		Operation:         op,
		OperationPath:     path,
	}
	return encodeOperationEnvelope("addon.install.prepare", data)
}

func commitAddonInstall(fields map[string]string, timeout time.Duration) (string, error) {
	op, err := verifyPreparedOperation(fields, "addon_install")
	if err != nil {
		return "", err
	}

	release, err := acquireEnvLock(op)
	if err != nil {
		return "", err
	}
	defer release()

	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "addon_install_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      map[string]string{"env": op.Target.Env, "addon": op.Target.Addon},
		Result:      "started",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}

	op.Status = "RUNNING"
	op.StartedAt = time.Now().UTC().Format(time.RFC3339)
	op.Approval.Status = "approved"
	markStep(&op, "collect pre-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "pre-snapshot.json", op.Target.Env); err != nil {
		op.Status = "FAILED"
		op.ErrorCode = "SNAPSHOT_FAILED"
		op.Error = err.Error()
		markStep(&op, "collect pre-snapshot", "failed")
		_, _ = writeOperation(op)
		return "", err
	}
	markStep(&op, "collect pre-snapshot", "succeeded")
	markStep(&op, "run addon install", "running")
	_, _ = writeOperation(op)

	err = runAddonCommand(op, timeout)
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		op.Status = "FAILED"
		op.ErrorCode = "COMMAND_FAILED"
		op.Error = err.Error()
		markStep(&op, "run addon install", "failed")
		_, _ = writeOperation(op)
		_, _ = appendAudit(profileAuditRecord{
			Time:        time.Now().UTC().Format(time.RFC3339),
			OperationID: op.OperationID,
			Tool:        "addon_install_commit",
			Actor:       "agent",
			Risk:        op.Risk,
			Target:      map[string]string{"env": op.Target.Env, "addon": op.Target.Addon},
			Result:      "failed",
		})
		return "", err
	}
	op.Status = "SUCCEEDED"
	markStep(&op, "run addon install", "succeeded")
	markStep(&op, "run addon verify", "succeeded")
	markStep(&op, "collect post-snapshot", "running")
	_, _ = writeOperation(op)
	if err := saveOperationSnapshot(op.OperationID, "post-snapshot.json", op.Target.Env); err != nil {
		op.Status = "FAILED"
		op.ErrorCode = "SNAPSHOT_FAILED"
		op.Error = err.Error()
		markStep(&op, "collect post-snapshot", "failed")
		_, _ = writeOperation(op)
		return "", err
	}
	markStep(&op, "collect post-snapshot", "succeeded")
	_, _ = writeOperation(op)
	_, _ = appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "addon_install_commit",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      map[string]string{"env": op.Target.Env, "addon": op.Target.Addon},
		Result:      "ok",
	})
	return encodeOperationEnvelope("addon.install.commit", op)
}

func verifyPreparedOperation(fields map[string]string, tool string) (operationRecord, error) {
	operationID := fields["operationId"]
	token := fields["approvalToken"]
	op, err := readOperation(operationID)
	if err != nil {
		return op, err
	}
	if op.Tool != tool {
		return op, fmt.Errorf("operation tool mismatch: %s", op.Tool)
	}
	if op.Status != "PREPARED" && op.Status != "APPROVED" {
		return op, fmt.Errorf("operation status must be PREPARED or APPROVED, got %s", op.Status)
	}
	expiresAt, err := time.Parse(time.RFC3339, op.ExpiresAt)
	if err != nil {
		return op, err
	}
	if time.Now().UTC().After(expiresAt) {
		op.Status = "EXPIRED"
		_, _ = writeOperation(op)
		return op, fmt.Errorf("APPROVAL_TOKEN_EXPIRED: operation expired")
	}
	if op.Status == "APPROVED" && op.Approval.Status == "approved" && token == "" {
		return op, nil
	}
	expected, err := approvalToken(op)
	if err != nil {
		return op, err
	}
	if token == "" {
		return op, fmt.Errorf("APPROVAL_REQUIRED")
	}
	if !hmac.Equal([]byte(expected), []byte(token)) {
		return op, fmt.Errorf("APPROVAL_TOKEN_INVALID")
	}
	return op, nil
}

func failOperation(op operationRecord, code string, err error, step string) (string, error) {
	op.Status = "FAILED"
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	op.ErrorCode = code
	op.Error = err.Error()
	markStep(&op, step, "failed")
	_, _ = writeOperation(op)
	return "", err
}

func saveOperationSnapshot(operationID, name, env string) error {
	dir, err := operationDir(operationID)
	if err != nil {
		return err
	}
	raw, err := collectSnapshot("snapshot.collect", env, 90*time.Second)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(raw), 0644)
}

func runEnvUpCommand(op operationRecord, timeout time.Duration) error {
	root, err := infraLabRoot()
	if err != nil {
		return err
	}
	dir, err := operationDir(op.OperationID)
	if err != nil {
		return err
	}
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer stderr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "bin", "ilab"), "env", "up", op.Target.Profile)
	cmd.Env = append(os.Environ(), "INFRA_LAB_ROOT="+root)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("COMMAND_TIMEOUT: env up timed out after %s", timeout)
		}
		return err
	}
	return nil
}

func runDestructiveCommand(op operationRecord, timeout time.Duration) error {
	root, err := infraLabRoot()
	if err != nil {
		return err
	}
	switch op.Tool {
	case "env_down":
		return runLoggedCommand(op, timeout, filepath.Join(root, "bin", "ilab"), "env", "down", op.Target.Env)
	case "env_clean":
		return runLoggedCommandWithEnv(op, timeout, cleanEnvVars(op.Target.Env), "bash", filepath.Join(root, "scripts", "k8s-tool.sh"), "clean")
	case "env_rebuild":
		return runLoggedCommand(op, timeout, filepath.Join(root, "bin", "ilab"), "env", "rebuild", op.Target.Profile, "--approve")
	case "addon_uninstall":
		return runLoggedCommand(op, timeout, "bash", filepath.Join(root, "scripts", "k8s-tool.sh"), "addons-uninstall", addonScope(op.Target.Addon), op.Target.Addon)
	default:
		return fmt.Errorf("unsupported destructive tool: %s", op.Tool)
	}
}

func cleanEnvVars(env string) map[string]string {
	return map[string]string{
		"FORCE":    "1",
		"ENV_NAME": env,
	}
}

func runAddonCommand(op operationRecord, timeout time.Duration) error {
	root, err := infraLabRoot()
	if err != nil {
		return err
	}
	scope := addonScope(op.Target.Addon)
	for _, args := range [][]string{
		{"addons-install", scope, op.Target.Addon},
		{"addons-verify", scope, op.Target.Addon},
	} {
		if err := runLoggedCommand(op, timeout, "bash", append([]string{filepath.Join(root, "scripts", "k8s-tool.sh")}, args...)...); err != nil {
			return err
		}
	}
	return nil
}

func addonScope(addon string) string {
	if addon == "metrics-server" {
		return "base"
	}
	return "optional"
}

func runLoggedCommand(op operationRecord, timeout time.Duration, command string, args ...string) error {
	return runLoggedCommandWithEnv(op, timeout, nil, command, args...)
}

func runLoggedCommandWithEnv(op operationRecord, timeout time.Duration, extraEnv map[string]string, command string, args ...string) error {
	root, err := infraLabRoot()
	if err != nil {
		return err
	}
	dir, err := operationDir(op.OperationID)
	if err != nil {
		return err
	}
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer stderr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = append(os.Environ(), "INFRA_LAB_ROOT="+root, "ENV_NAME="+op.Target.Env)
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("COMMAND_TIMEOUT: operation command timed out after %s", timeout)
		}
		return err
	}
	return nil
}

func operationStatus(operationID string) (string, error) {
	op, err := readOperation(operationID)
	if err != nil {
		return "", err
	}
	return encodeOperationEnvelope("operation.status", op)
}

func operationLogs(operationID string) (string, error) {
	dir, err := operationDir(operationID)
	if err != nil {
		return "", err
	}
	stdout, _ := os.ReadFile(filepath.Join(dir, "stdout.log"))
	stderr, _ := os.ReadFile(filepath.Join(dir, "stderr.log"))
	return encodeOperationEnvelope("operation.logs", operationLogsData{
		OperationID: operationID,
		Stdout:      string(stdout),
		Stderr:      string(stderr),
	})
}

func operationApprove(operationID string) (string, error) {
	op, err := readOperation(operationID)
	if err != nil {
		return "", err
	}
	if op.Status != "PREPARED" {
		return "", fmt.Errorf("operation status must be PREPARED, got %s", op.Status)
	}
	if op.Approval.Required {
		op.Approval.Status = "approved"
	}
	op.Status = "APPROVED"
	if _, err := writeOperation(op); err != nil {
		return "", err
	}
	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "operation_approve",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "ok",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}
	return encodeOperationEnvelope("operation.approve", op)
}

func operationCancel(operationID string) (string, error) {
	op, err := readOperation(operationID)
	if err != nil {
		return "", err
	}
	switch op.Status {
	case "PREPARED", "APPROVED":
	default:
		return "", fmt.Errorf("operation status must be PREPARED or APPROVED, got %s", op.Status)
	}
	op.Status = "CANCELLED"
	op.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if op.Approval.Status == "required" || op.Approval.Status == "approved" {
		op.Approval.Status = "rejected"
	}
	if _, err := writeOperation(op); err != nil {
		return "", err
	}
	if _, err := appendAudit(profileAuditRecord{
		Time:        time.Now().UTC().Format(time.RFC3339),
		OperationID: op.OperationID,
		Tool:        "operation_cancel",
		Actor:       "agent",
		Risk:        op.Risk,
		Target:      auditTarget(op),
		Result:      "ok",
	}); err != nil {
		return "", fmt.Errorf("AUDIT_WRITE_FAILED: %w", err)
	}
	return encodeOperationEnvelope("operation.cancel", op)
}

func operationLocks() (string, error) {
	dir, err := lockDir()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return encodeOperationEnvelope("operation.locks", operationLocksData{Locks: []operationLockRecord{}})
		}
		return "", err
	}
	locks := []operationLockRecord{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		lock, err := readLock(path)
		if err != nil {
			continue
		}
		locks = append(locks, lock)
	}
	sort.Slice(locks, func(i, j int) bool {
		return locks[i].Env < locks[j].Env
	})
	return encodeOperationEnvelope("operation.locks", operationLocksData{Locks: locks})
}

func operationUnlockStale(env string) (string, error) {
	if env == "" {
		return "", fmt.Errorf("env is required")
	}
	if err := validateTokenPart(env); err != nil {
		return "", err
	}
	dir, err := lockDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, env+".lock")
	lock, err := readLock(path)
	if err != nil {
		return "", err
	}
	if !lock.Stale {
		return "", fmt.Errorf("LOCK_NOT_STALE: %s", env)
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	data := map[string]any{
		"env":     env,
		"removed": true,
		"lock":    lock,
	}
	return encodeOperationEnvelope("operation.unlock_stale", data)
}

func readLock(path string) (operationLockRecord, error) {
	var lock operationLockRecord
	data, err := os.ReadFile(path)
	if err != nil {
		return lock, err
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return lock, err
	}
	lock.Path = path
	if expiresAt, err := time.Parse(time.RFC3339, lock.ExpiresAt); err == nil {
		lock.Stale = time.Now().UTC().After(expiresAt)
	}
	return lock, nil
}

func writeOperation(op operationRecord) (string, error) {
	dir, err := operationDir(op.OperationID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "operation.json")
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

func readOperation(operationID string) (operationRecord, error) {
	var op operationRecord
	if operationID == "" {
		return op, fmt.Errorf("operationId is required")
	}
	dir, err := operationDir(operationID)
	if err != nil {
		return op, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "operation.json"))
	if err != nil {
		return op, fmt.Errorf("OPERATION_NOT_FOUND: %s", operationID)
	}
	if err := json.Unmarshal(data, &op); err != nil {
		return op, err
	}
	return op, nil
}

func operationDir(operationID string) (string, error) {
	base, err := operationStoreDir()
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(operationID, `/\`) || operationID == "" {
		return "", fmt.Errorf("invalid operationId: %q", operationID)
	}
	return filepath.Join(base, operationID), nil
}

func operationStoreDir() (string, error) {
	if dir := os.Getenv("INFRA_LAB_OPERATION_STORE"); dir != "" {
		return dir, nil
	}
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", ".operations"), nil
}

func acquireEnvLock(op operationRecord) (func(), error) {
	if op.Target.Env == "" {
		return nil, fmt.Errorf("env is required for env lock")
	}
	return acquireNamedLock(op.Target.Env, op)
}

func acquireNamedLock(name string, op operationRecord) (func(), error) {
	if err := validateTokenPart(name); err != nil {
		return nil, err
	}
	dir, err := lockDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("LOCK_HELD: %s", name)
		}
		return nil, err
	}
	lock := map[string]any{
		"operationId": op.OperationID,
		"env":         op.Target.Env,
		"tool":        op.Tool + "_commit",
		"startedAt":   time.Now().UTC().Format(time.RFC3339),
		"expiresAt":   time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
		"pid":         os.Getpid(),
	}
	if op.Target.Name != "" {
		lock["name"] = op.Target.Name
	}
	if op.Target.Image != "" {
		lock["image"] = op.Target.Image
	}
	if hostname, err := os.Hostname(); err == nil {
		lock["hostname"] = hostname
	}
	data, _ := json.MarshalIndent(lock, "", "  ")
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return nil, writeErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return nil, closeErr
	}
	return func() { _ = os.Remove(path) }, nil
}

func lockDir() (string, error) {
	if dir := os.Getenv("INFRA_LAB_LOCK_DIR"); dir != "" {
		return dir, nil
	}
	root, err := infraLabRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", ".locks"), nil
}

func approvalToken(op operationRecord) (string, error) {
	secret, err := localSecret()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	for _, part := range []string{op.OperationID, op.Tool, op.Target.Env, op.Target.Addon, op.Target.Profile, op.Target.ProfileDigest, op.Target.TargetFingerprint, op.Risk, op.ExpiresAt} {
		_, _ = mac.Write([]byte(part))
		_, _ = mac.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(mac.Sum(nil)), nil
}

func localSecret() ([]byte, error) {
	path, err := secretPath()
	if err != nil {
		return nil, err
	}
	if data, err := os.ReadFile(path); err == nil {
		return bytes.TrimSpace(data), nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	encoded := []byte(hex.EncodeToString(secret) + "\n")
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(encoded), nil
}

func secretPath() (string, error) {
	if path := os.Getenv("INFRA_LAB_MCP_SECRET_PATH"); path != "" {
		return path, nil
	}
	configHome := infraLabConfigHome()
	if configHome == "" {
		return "", fmt.Errorf("SECRET_NOT_FOUND: config home not found")
	}
	return filepath.Join(configHome, "mcp", "secret"), nil
}

func markStep(op *operationRecord, name, status string) {
	for i := range op.Steps {
		if op.Steps[i].Name == name {
			op.Steps[i].Status = status
			return
		}
	}
}

func destructiveSteps(tool string) []operationStep {
	names := []string{"collect pre-snapshot", destructiveRunStep(tool), "collect post-snapshot"}
	out := make([]operationStep, 0, len(names))
	for _, name := range names {
		out = append(out, operationStep{Name: name, Status: "pending"})
	}
	return out
}

func destructiveRunStep(tool string) string {
	switch tool {
	case "env_down":
		return "run env down"
	case "env_clean":
		return "run env clean"
	case "env_rebuild":
		return "run env rebuild"
	case "addon_uninstall":
		return "run addon uninstall"
	default:
		return "run destructive command"
	}
}

func commandForTool(tool string) string {
	switch tool {
	case "env_down":
		return "env.down"
	case "env_clean":
		return "env.clean"
	case "env_rebuild":
		return "env.rebuild"
	case "addon_uninstall":
		return "addon.uninstall"
	default:
		return strings.ReplaceAll(tool, "_", ".")
	}
}

func auditTarget(op operationRecord) map[string]string {
	target := map[string]string{"env": op.Target.Env}
	if op.Target.Name != "" {
		target["name"] = op.Target.Name
	}
	if op.Target.Profile != "" {
		target["profile"] = op.Target.Profile
	}
	if op.Target.Addon != "" {
		target["addon"] = op.Target.Addon
	}
	if op.Target.VM != "" {
		target["vm"] = op.Target.VM
	}
	if op.Target.Image != "" {
		target["image"] = op.Target.Image
	}
	if op.Target.ContextDir != "" {
		target["contextDir"] = op.Target.ContextDir
	}
	return target
}

func encodeOperationEnvelope(command string, data any) (string, error) {
	env := operationEnvelope{
		OK:              true,
		Command:         command,
		ContractVersion: supportedContractVersion,
		Data:            data,
		Warnings:        []any{},
		Errors:          []interface{}{},
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

func validateTokenPart(value string) error {
	if strings.ContainsAny(value, `/\`) || strings.TrimSpace(value) != value {
		return fmt.Errorf("invalid value: %q", value)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
