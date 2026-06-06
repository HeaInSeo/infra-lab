package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Profile management",
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	RunE:  runProfileList,
}

var profileCloneCmd = &cobra.Command{
	Use:   "clone <src> <dst>",
	Short: "Clone a profile under a new name",
	Args:  cobra.ExactArgs(2),
	RunE:  runProfileClone,
}

func init() {
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileCloneCmd)
}

// runProfileList lists profiles from ~/.config/infra-lab/profiles/ and <repo>/envs/*.yaml.
func runProfileList(_ *cobra.Command, _ []string) error {
	var profiles []string

	// ~/.config/infra-lab/profiles/
	home, _ := os.UserHomeDir()
	if home != "" {
		userDir := filepath.Join(home, ".config", "infra-lab", "profiles")
		if entries, err := os.ReadDir(userDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
					profiles = append(profiles, filepath.Join(userDir, e.Name()))
				}
			}
		}
	}

	// <repo>/envs/*.yaml
	root, err := lab.FindRoot()
	if err == nil {
		envsDir := filepath.Join(root, "envs")
		if entries, err := os.ReadDir(envsDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") &&
					!strings.HasSuffix(e.Name(), ".yaml.example") {
					profiles = append(profiles, filepath.Join(envsDir, e.Name()))
				}
			}
		}
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles found.")
		fmt.Println("hint: copy an example from envs/*.yaml.example and fill in your values.")
		return nil
	}

	for _, p := range profiles {
		fmt.Println(p)
	}
	return nil
}

// runProfileClone copies src profile to dst, updating name and state.dir.
func runProfileClone(_ *cobra.Command, args []string) error {
	srcArg, dstArg := args[0], args[1]

	// Load source profile.
	src, err := lab.LoadProfile(srcArg)
	if err != nil {
		return fmt.Errorf("load source profile: %w", err)
	}

	// Determine destination name (stem of dstArg).
	dstStem := dstArg
	dstStem = strings.TrimSuffix(dstStem, ".yaml")
	dstStem = strings.TrimSuffix(dstStem, ".yaml.example")
	dstStem = filepath.Base(dstStem) // strip any directory component for the name

	// Determine destination file path.
	dstPath, err := resolveDstProfilePath(dstArg)
	if err != nil {
		return err
	}

	// Refuse to overwrite an existing file.
	if _, err := os.Stat(dstPath); err == nil {
		return fmt.Errorf("destination already exists: %s", dstPath)
	}

	// Shallow-copy profile and update name / state.dir.
	dst := *src
	dst.Name = dstStem
	dst.State.Dir = "state/" + dstStem

	data, err := yaml.Marshal(&dst)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}

	fmt.Printf("Profile cloned: %s -> %s\n", src.Name, dstPath)
	return nil
}

// resolveDstProfilePath determines where to write the cloned profile.
//
// Priority:
//  1. dstArg is absolute → use as-is
//  2. dstArg contains path separator → use relative to cwd
//  3. otherwise → <repo>/envs/<dstArg>.yaml (or ~/.config/... if no repo found)
func resolveDstProfilePath(dstArg string) (string, error) {
	// Already an absolute path.
	if filepath.IsAbs(dstArg) {
		if !strings.HasSuffix(dstArg, ".yaml") {
			dstArg += ".yaml"
		}
		return dstArg, nil
	}

	// Contains directory component (relative path).
	if strings.ContainsRune(dstArg, filepath.Separator) {
		abs, err := filepath.Abs(dstArg)
		if err != nil {
			return "", err
		}
		if !strings.HasSuffix(abs, ".yaml") {
			abs += ".yaml"
		}
		return abs, nil
	}

	// Plain name: prefer <repo>/envs/.
	stem := strings.TrimSuffix(dstArg, ".yaml")
	root, err := lab.FindRoot()
	if err == nil {
		return filepath.Join(root, "envs", stem+".yaml"), nil
	}

	// Fallback: ~/.config/infra-lab/profiles/.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "infra-lab", "profiles", stem+".yaml"), nil
}
