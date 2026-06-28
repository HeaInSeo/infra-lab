package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
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

var profileShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Display profile contents",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileShow,
}

var profileValidateCmd = &cobra.Command{
	Use:   "validate <name>",
	Short: "Validate profile fields",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileValidate,
}

// profile new flags
var (
	profileNewBackend  string
	profileNewCNI      string
	profileNewWorkers  int
	profileNewMasters  int
	profileNewOS       string
	profileNewSSHKey   string
	profileNewPoolName string
	profileNewPoolPath string
)

var profileNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new profile from flags",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileNew,
}

func init() {
	profileNewCmd.Flags().StringVar(&profileNewBackend, "backend", "libvirt", "backend (libvirt|multipass)")
	profileNewCmd.Flags().StringVar(&profileNewCNI, "cni", "flannel", "CNI plugin (flannel|cilium|calico)")
	profileNewCmd.Flags().IntVar(&profileNewWorkers, "workers", 2, "number of worker nodes")
	profileNewCmd.Flags().IntVar(&profileNewMasters, "masters", 1, "number of control-plane nodes")
	profileNewCmd.Flags().StringVar(&profileNewOS, "os", "ubuntu-24.04", fmt.Sprintf("OS image name (%s)", strings.Join(lab.SupportedOSImages(), "|")))
	profileNewCmd.Flags().StringVar(&profileNewSSHKey, "ssh-key", "~/.ssh/id_ed25519", "SSH private key path (libvirt only)")
	profileNewCmd.Flags().StringVar(&profileNewPoolName, "pool-name", "lab-pool", "libvirt pool name")
	profileNewCmd.Flags().StringVar(&profileNewPoolPath, "pool-path", "/var/lib/libvirt/images", "libvirt pool path")

	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileCloneCmd)
	profileCmd.AddCommand(profileNewCmd)
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileValidateCmd)
}

// runProfileNew creates a new profile YAML from flags and saves it to
// ~/.config/infra-lab/profiles/<name>.yaml.
func runProfileNew(_ *cobra.Command, args []string) error {
	name := args[0]

	imageURL := lab.OSImageURL(profileNewOS)
	if imageURL == "" {
		fmt.Fprintf(os.Stderr, "warning: unknown OS image %q — vm.imageUrl will be blank; fill it in manually\n", profileNewOS)
	}

	p := &lab.Profile{
		Name:    name,
		Backend: profileNewBackend,
		VM: lab.VMSpec{
			OSImage:  profileNewOS,
			ImageURL: imageURL,
			Masters:  profileNewMasters,
			Workers:  profileNewWorkers,
			Master:   lab.NodeSpec{CPU: 2, Memory: "4G", Disk: "40G"},
			Worker:   lab.NodeSpec{CPU: 2, Memory: "4G", Disk: "50G"},
		},
		Kubernetes: lab.KubernetesSpec{
			Version: "1.32",
			CNI:     profileNewCNI,
		},
		Addons: lab.AddonsSpec{
			Base:     []string{"metrics-server"},
			Optional: []string{},
		},
		State: lab.StateSpec{Dir: "state/" + name},
	}

	if profileNewBackend == "libvirt" {
		sshPub, err := readSSHPublicKey(profileNewSSHKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not read public key from %s.pub: %v\n", profileNewSSHKey, err)
			sshPub = "ssh-ed25519 AAAA... # TODO: replace with your public key"
		}
		p.Libvirt = &lab.LibvirtSpec{
			SSHPrivateKeyPath: profileNewSSHKey,
			SSHPublicKey:      sshPub,
			PoolName:          profileNewPoolName,
			PoolPath:          profileNewPoolPath,
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "infra-lab", "profiles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create profiles dir: %w", err)
	}
	outPath := filepath.Join(dir, name+".yaml")

	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("profile already exists: %s\nhint: use 'ilab profile clone %s <new-name>' to create a variant", outPath, name)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0600); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}

	fmt.Printf("Profile created: %s\n\n", outPath)
	fmt.Printf("Next steps:\n")
	fmt.Printf("  ilab profile validate %s\n", name)
	fmt.Printf("  ilab env up %s\n", name)
	return nil
}

// runProfileShow pretty-prints the YAML for a named profile.
func runProfileShow(_ *cobra.Command, args []string) error {
	p, err := loadProfileForOutput(args[0])
	if err != nil {
		return err
	}
	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("profile.show", profileShowData{
			Profile: profileRef(args[0], p),
			Spec:    profileNormalized(p),
		}))
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

// runProfileValidate validates a profile's required fields and prints errors.
func runProfileValidate(_ *cobra.Command, args []string) error {
	p, err := loadProfileForOutput(args[0])
	if err != nil {
		return err
	}
	errs := p.Validate()
	if wantsJSON() {
		conditions := []conditionData{{
			Type:   "SchemaValid",
			Status: "True",
			Reason: "ValidationPassed",
		}}
		if len(errs) > 0 {
			conditions = []conditionData{{
				Type:    "SchemaValid",
				Status:  "False",
				Reason:  "ValidationFailed",
				Message: "profile validation failed",
			}}
			return &output.ContractError{
				Code:    "PROFILE_INVALID",
				Message: "profile validation failed",
				Exit:    output.ExitDomain,
				Infos:   contractErrors("PROFILE_INVALID", errs),
			}
		}
		return output.WriteJSON(os.Stdout, output.Success("profile.validate", profileValidateData{
			Profile:    profileRef(args[0], p),
			Valid:      true,
			Normalized: profileNormalized(p),
			Conditions: conditions,
		}))
	}
	if len(errs) == 0 {
		fmt.Printf("Profile %q is valid.\n", p.Name)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Profile %q has %d error(s):\n", p.Name, len(errs))
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "  - %s\n", e)
	}
	return fmt.Errorf("profile validation failed")
}

func loadProfileForOutput(arg string) (*lab.Profile, error) {
	if !wantsJSON() {
		return lab.LoadProfile(arg)
	}
	location, err := lab.ResolveProfileLocation(arg)
	if err != nil {
		return nil, output.WrapError("PROFILE_NOT_FOUND", err.Error(), output.ExitDomain, err)
	}
	return lab.LoadProfile(location.Path)
}

// readSSHPublicKey reads the public key from <privKeyPath>.pub.
func readSSHPublicKey(privKeyPath string) (string, error) {
	expanded := lab.ExpandTilde(privKeyPath)
	data, err := os.ReadFile(expanded + ".pub")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// runProfileList lists profiles from ~/.config/infra-lab/profiles/ and <repo>/envs/*.yaml.
func runProfileList(_ *cobra.Command, _ []string) error {
	profiles := listProfiles()

	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("profile.list", profileListData{Profiles: profiles}))
	}

	var paths []string
	for _, p := range profiles {
		paths = append(paths, p.Path)
	}

	if len(paths) == 0 {
		fmt.Println("No profiles found.")
		fmt.Println("hint: copy an example from envs/*.yaml.example and fill in your values.")
		return nil
	}

	for _, p := range paths {
		fmt.Println(p)
	}
	return nil
}

func listProfiles() []profileListItemData {
	profiles := []profileListItemData{}

	home, _ := os.UserHomeDir()
	if home != "" {
		userDir := filepath.Join(home, ".config", "infra-lab", "profiles")
		if entries, err := os.ReadDir(userDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
					path := filepath.Join(userDir, e.Name())
					profiles = append(profiles, profileListItemData{
						Name:   strings.TrimSuffix(e.Name(), ".yaml"),
						Source: "user",
						Path:   path,
					})
				}
			}
		}
	}

	root, err := lab.FindRoot()
	if err == nil {
		envsDir := filepath.Join(root, "envs")
		if entries, err := os.ReadDir(envsDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") &&
					!strings.HasSuffix(e.Name(), ".yaml.example") {
					path := filepath.Join(envsDir, e.Name())
					profiles = append(profiles, profileListItemData{
						Name:   strings.TrimSuffix(e.Name(), ".yaml"),
						Source: "repo",
						Path:   path,
					})
				}
			}
		}
	}

	return profiles
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
