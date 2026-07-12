package lab

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const lowDiskKiB = 1024 * 1024

type DiagnosticFinding struct {
	Code    string
	Message string
}

func DiagnoseLibvirtVM(vm VMInfo) []DiagnosticFinding {
	if vm.Backend != "libvirt" {
		return nil
	}
	var findings []DiagnosticFinding
	state := strings.ToLower(vm.State)
	if strings.Contains(state, "paused") || strings.Contains(state, "일시") {
		findings = append(findings, DiagnosticFinding{
			Code:    "LIBVIRT_VM_PAUSED",
			Message: fmt.Sprintf("libvirt VM %q is paused; check block I/O errors and host storage before resuming", vm.Name),
		})
	}
	if errors := libvirtBlockErrors(vm.Name); len(errors) > 0 {
		findings = append(findings, DiagnosticFinding{
			Code:    "LIBVIRT_IO_ERROR",
			Message: fmt.Sprintf("libvirt VM %q reports block I/O error(s): %s", vm.Name, strings.Join(errors, "; ")),
		})
		for _, errText := range errors {
			lower := strings.ToLower(errText)
			if strings.Contains(lower, "no space") || strings.Contains(lower, "nospace") || strings.Contains(lower, "enospc") {
				findings = append(findings, DiagnosticFinding{
					Code:    "HOST_NOSPACE",
					Message: fmt.Sprintf("libvirt VM %q has a block error consistent with host storage exhaustion: %s", vm.Name, errText),
				})
				break
			}
		}
	}
	for _, volume := range libvirtBlockPaths(vm.Name) {
		if free, ok := filesystemFreeKiB(volume); ok && free < lowDiskKiB {
			findings = append(findings, DiagnosticFinding{
				Code:    "HOST_NOSPACE",
				Message: fmt.Sprintf("filesystem backing %s for libvirt VM %q has only %s free", volume, vm.Name, formatKiB(free)),
			})
		}
	}
	return findings
}

func libvirtBlockErrors(vmName string) []string {
	out, err := exec.Command("virsh", "-c", libvirtURI, "domblkerror", vmName).Output()
	if err != nil {
		return nil
	}
	return ParseLibvirtBlockErrors(string(out))
}

func libvirtBlockPaths(vmName string) []string {
	out, err := exec.Command("virsh", "-c", libvirtURI, "domblklist", vmName, "--details").Output()
	if err != nil {
		return nil
	}
	return ParseLibvirtBlockPaths(string(out))
}

func filesystemFreeKiB(path string) (int64, bool) {
	target := path
	if filepath.Ext(path) != "" {
		target = filepath.Dir(path)
	}
	out, err := exec.Command("df", "-Pk", target).Output()
	if err != nil {
		return 0, false
	}
	return ParseDFFreeKiB(string(out))
}

func ParseLibvirtBlockErrors(out string) []string {
	var errors []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "no errors") ||
			strings.Contains(lower, "no error") ||
			strings.Contains(line, "오류 메세지를 찾을 수 없음") ||
			strings.Contains(line, "오류 메시지를 찾을 수 없음") {
			continue
		}
		errors = append(errors, line)
	}
	return errors
}

func ParseLibvirtBlockPaths(out string) []string {
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		source := fields[len(fields)-1]
		if strings.HasPrefix(source, "/") {
			paths = append(paths, source)
		}
	}
	return paths
}

func ParseDFFreeKiB(out string) (int64, bool) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return 0, false
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return 0, false
	}
	free, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return 0, false
	}
	return free, true
}

func formatKiB(kib int64) string {
	if kib >= 1024*1024 {
		return fmt.Sprintf("%.1f GiB", float64(kib)/(1024*1024))
	}
	if kib >= 1024 {
		return fmt.Sprintf("%.1f MiB", float64(kib)/1024)
	}
	return fmt.Sprintf("%d KiB", kib)
}
