package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/HeaInSeo/infra-lab/ilab/internal/contract"
	"github.com/HeaInSeo/infra-lab/ilab/internal/lab"
	"github.com/HeaInSeo/infra-lab/ilab/internal/output"
)

// observer produces a live reading of the cluster and Harbor front door. It is
// injected so command logic stays testable with fixtures instead of a live
// cluster.
type observer interface {
	Observe(root, envName string) (contract.Observation, error)
}

// resolveFormat is the non-JSON output form for `ilab context resolve`.
var resolveFormat string

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Runtime context contract: capture, resolve, invalidate",
	Long: `context manages state/<env>/runtime-contract.json — a secret-free record of
what infra-lab observed live (cluster identity, Harbor hostname, TLS CA
fingerprint) when an environment was stood up.

Consumers never hardcode an endpoint; they run 'ilab context resolve', which
reconciles the stored contract against the live cluster/TLS and only emits an
endpoint when they still agree. A missing or mismatched contract fails closed.`,
}

var contextCaptureCmd = &cobra.Command{
	Use:   "capture [env-name]",
	Short: "Observe the live cluster/Harbor and write the runtime contract",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runContextCapture,
}

var contextResolveCmd = &cobra.Command{
	Use:   "resolve [env-name]",
	Short: "Reconcile the runtime contract against the live cluster and emit endpoints",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runContextResolve,
}

var contextInvalidateCmd = &cobra.Command{
	Use:   "invalidate [env-name]",
	Short: "Remove the runtime contract (call on cluster/Harbor teardown or rebuild)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runContextInvalidate,
}

func init() {
	contextResolveCmd.Flags().StringVar(&resolveFormat, "format", "text",
		"non-JSON output form: text|shell (use --json for the JSON envelope)")
	contextCmd.AddCommand(contextCaptureCmd)
	contextCmd.AddCommand(contextResolveCmd)
	contextCmd.AddCommand(contextInvalidateCmd)
	rootCmd.AddCommand(contextCmd)
}

func argEnv(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// ── capture ─────────────────────────────────────────────────────────────────

type contractCaptureData struct {
	Env             string `json:"env"`
	ContractPath    string `json:"contractPath"`
	ContractDigest  string `json:"contractDigest"`
	ClusterIdentity string `json:"clusterIdentity"`
	HarborHostname  string `json:"harborHostname"`
	HarborGateway   string `json:"harborGateway"`
	ObservedAt      string `json:"observedAt"`
}

func runContextCapture(_ *cobra.Command, args []string) error {
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	data, err := captureContract(root, argEnv(args), kubectlObserver{}, time.Now)
	if err != nil {
		return err
	}
	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("context.capture", data))
	}
	fmt.Printf("wrote runtime contract: %s\n", data.ContractPath)
	fmt.Printf("  clusterIdentity: %s\n", data.ClusterIdentity)
	fmt.Printf("  harbor:          %s (%s)\n", data.HarborHostname, data.HarborGateway)
	fmt.Printf("  contractDigest:  %s\n", data.ContractDigest)
	return nil
}

func captureContract(root, envName string, obs observer, now func() time.Time) (contractCaptureData, error) {
	if envName == "" {
		return contractCaptureData{}, output.NewError(
			"ENV_REQUIRED", "capture requires an explicit env-name", output.ExitUsage)
	}
	observation, err := obs.Observe(root, envName)
	if err != nil {
		return contractCaptureData{}, err
	}
	c := contract.FromObservation(envName, currentVersionData().GitCommit, observation, now())
	if err := c.Validate(); err != nil {
		return contractCaptureData{}, output.WrapError(
			"CONTRACT_INCOMPLETE", err.Error(), output.ExitDomain, err)
	}
	path := contract.Path(root, envName)
	digest, err := contract.Write(path, c)
	if err != nil {
		return contractCaptureData{}, output.WrapError(
			"CONTRACT_WRITE_FAILED", err.Error(), output.ExitRuntime, err)
	}
	return contractCaptureData{
		Env:             envName,
		ContractPath:    path,
		ContractDigest:  digest,
		ClusterIdentity: c.Kubernetes.ClusterIdentity,
		HarborHostname:  c.Harbor.Hostname,
		HarborGateway:   c.Harbor.Gateway,
		ObservedAt:      c.ObservedAt,
	}, nil
}

// ── resolve ─────────────────────────────────────────────────────────────────

type contractResolveData struct {
	Env               string `json:"env"`
	Kubeconfig        string `json:"kubeconfig"`
	Context           string `json:"context"`
	HarborHostname    string `json:"harborHostname"`
	HarborExternalURL string `json:"harborExternalURL"`
	HarborGateway     string `json:"harborGateway"`
	CAFile            string `json:"caFile"`
	Reconciled        bool   `json:"reconciled"`
	ContractDigest    string `json:"contractDigest"`
	ResolveDigest     string `json:"resolveDigest"`
}

func runContextResolve(_ *cobra.Command, args []string) error {
	if wantsJSON() && resolveFormat != "text" {
		return output.NewError(
			"FORMAT_CONFLICT", "--json cannot be combined with --format", output.ExitUsage)
	}
	if resolveFormat != "text" && resolveFormat != "shell" {
		return output.NewError(
			"FORMAT_UNSUPPORTED",
			fmt.Sprintf("unsupported --format %q (want text|shell)", resolveFormat),
			output.ExitUsage)
	}
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	data, err := resolveContract(root, argEnv(args), kubectlObserver{})
	if err != nil {
		return err
	}
	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("context.resolve", data))
	}
	if resolveFormat == "shell" {
		return writeResolveShell(os.Stdout, data)
	}
	return writeResolveText(os.Stdout, data)
}

// resolveContract loads the stored contract, reconciles it against a live
// observation, and returns resolved endpoints only when they still agree.
// Every failure path is fail-closed: a non-nil error means no endpoint is ever
// handed to the caller.
func resolveContract(root, envName string, obs observer) (contractResolveData, error) {
	path := contract.Path(root, envName)
	c, raw, err := contract.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return contractResolveData{}, output.NewError(
				"CONTRACT_MISSING",
				fmt.Sprintf("no runtime contract at %s; run 'ilab context capture %s' on a live cluster", path, displayEnv(envName)),
				output.ExitDomain)
		}
		return contractResolveData{}, output.WrapError(
			"CONTRACT_UNREADABLE", err.Error(), output.ExitDomain, err)
	}
	if err := c.Validate(); err != nil {
		return contractResolveData{}, output.WrapError(
			"CONTRACT_INVALID", err.Error(), output.ExitDomain, err)
	}

	observation, err := obs.Observe(root, envName)
	if err != nil {
		// Cannot observe the live cluster -> cannot verify -> fail closed.
		return contractResolveData{}, err
	}

	if mismatches := contract.Reconcile(c, observation); len(mismatches) > 0 {
		infos := make([]output.ErrorInfo, 0, len(mismatches))
		for _, m := range mismatches {
			infos = append(infos, output.ErrorInfo{
				Code:    m.Code,
				Field:   m.Field,
				Message: fmt.Sprintf("contract %s=%q does not match live %q", m.Field, m.Expected, m.Actual),
			})
		}
		return contractResolveData{}, &output.ContractError{
			Code:    mismatches[0].Code,
			Message: "runtime contract does not match the live cluster; refusing to emit endpoints",
			Exit:    output.ExitDomain,
			Infos:   infos,
		}
	}

	data := contractResolveData{
		Env:               envName,
		Kubeconfig:        observation.KubeconfigPath,
		Context:           observation.Context,
		HarborHostname:    c.Harbor.Hostname,
		HarborExternalURL: c.Harbor.ExternalURL,
		HarborGateway:     c.Harbor.Gateway,
		CAFile:            c.Harbor.CAFile,
		Reconciled:        true,
		ContractDigest:    contract.Digest(raw),
	}
	data.ResolveDigest = resolveDigest(data)
	return data, nil
}

// resolveDigest fingerprints the resolved endpoint tuple as verification
// evidence. It is computed over the endpoint fields only (digests excluded) so
// it is stable and self-referential-free.
func resolveDigest(d contractResolveData) string {
	evidence := struct {
		Env               string `json:"env"`
		Kubeconfig        string `json:"kubeconfig"`
		Context           string `json:"context"`
		HarborHostname    string `json:"harborHostname"`
		HarborExternalURL string `json:"harborExternalURL"`
		HarborGateway     string `json:"harborGateway"`
		CAFile            string `json:"caFile"`
		ContractDigest    string `json:"contractDigest"`
	}{
		Env:               d.Env,
		Kubeconfig:        d.Kubeconfig,
		Context:           d.Context,
		HarborHostname:    d.HarborHostname,
		HarborExternalURL: d.HarborExternalURL,
		HarborGateway:     d.HarborGateway,
		CAFile:            d.CAFile,
		ContractDigest:    d.ContractDigest,
	}
	b, err := json.Marshal(evidence)
	if err != nil {
		return ""
	}
	return contract.Digest(b)
}

func writeResolveText(w io.Writer, d contractResolveData) error {
	fmt.Fprintf(w, "env:               %s\n", displayEnv(d.Env))
	fmt.Fprintf(w, "kubeconfig:        %s\n", d.Kubeconfig)
	fmt.Fprintf(w, "context:           %s\n", d.Context)
	fmt.Fprintf(w, "harbor hostname:   %s\n", d.HarborHostname)
	fmt.Fprintf(w, "harbor url:        %s\n", d.HarborExternalURL)
	fmt.Fprintf(w, "harbor gateway:    %s\n", d.HarborGateway)
	fmt.Fprintf(w, "harbor caFile:     %s\n", d.CAFile)
	fmt.Fprintf(w, "contractDigest:    %s\n", d.ContractDigest)
	fmt.Fprintf(w, "resolveDigest:     %s\n", d.ResolveDigest)
	return nil
}

// writeResolveshell emits sourceable export statements. It carries only
// endpoints and secret locators — never CA bytes or any secret value.
func writeResolveShell(w io.Writer, d contractResolveData) error {
	fmt.Fprintf(w, "# ilab context resolve — env=%s\n", displayEnv(d.Env))
	fmt.Fprintf(w, "# contractDigest=%s\n", d.ContractDigest)
	fmt.Fprintf(w, "# resolveDigest=%s\n", d.ResolveDigest)
	fmt.Fprintf(w, "export ILAB_KUBECONFIG=%s\n", shellQuote(d.Kubeconfig))
	fmt.Fprintf(w, "export ILAB_KUBE_CONTEXT=%s\n", shellQuote(d.Context))
	fmt.Fprintf(w, "export ILAB_HARBOR_HOSTNAME=%s\n", shellQuote(d.HarborHostname))
	fmt.Fprintf(w, "export ILAB_HARBOR_URL=%s\n", shellQuote(d.HarborExternalURL))
	fmt.Fprintf(w, "export ILAB_HARBOR_GATEWAY=%s\n", shellQuote(d.HarborGateway))
	fmt.Fprintf(w, "export ILAB_HARBOR_CA_FILE=%s\n", shellQuote(d.CAFile))
	return nil
}

func displayEnv(envName string) string {
	if envName == "" {
		return "<default>"
	}
	return envName
}

// ── invalidate ──────────────────────────────────────────────────────────────

type contractInvalidateData struct {
	Env       string `json:"env"`
	Path      string `json:"path"`
	Removed   bool   `json:"removed"`
	AlreadyAt string `json:"note,omitempty"`
}

func runContextInvalidate(_ *cobra.Command, args []string) error {
	envName := argEnv(args)
	if envName == "" {
		return output.NewError("ENV_REQUIRED", "invalidate requires an explicit env-name", output.ExitUsage)
	}
	root, err := lab.FindRoot()
	if err != nil {
		return err
	}
	path := contract.Path(root, envName)
	removed := true
	note := ""
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			return output.WrapError("CONTRACT_INVALIDATE_FAILED", err.Error(), output.ExitRuntime, err)
		}
		removed = false
		note = "no contract present"
	}
	data := contractInvalidateData{Env: envName, Path: path, Removed: removed, AlreadyAt: note}
	if wantsJSON() {
		return output.WriteJSON(os.Stdout, output.Success("context.invalidate", data))
	}
	if removed {
		fmt.Printf("invalidated runtime contract: %s\n", path)
	} else {
		fmt.Printf("no runtime contract to invalidate: %s\n", path)
	}
	return nil
}

// ── live observer ───────────────────────────────────────────────────────────

// kubectlObserver reads live cluster and Harbor state via kubectl. Every value
// is observed — none is defaulted or hardcoded. If any required reading fails,
// Observe returns an error and callers fail closed rather than guess.
type kubectlObserver struct{}

func (kubectlObserver) Observe(root, envName string) (contract.Observation, error) {
	kubeconfig, err := lab.ResolveKubeconfig(root, envName)
	if err != nil {
		return contract.Observation{}, output.WrapError(
			"KUBECONFIG_NOT_FOUND", err.Error(), output.ExitDomain, err)
	}

	uid, err := kubectlValue(kubeconfig, "get", "namespace", "kube-system",
		"-o", "jsonpath={.metadata.uid}")
	if err != nil || strings.TrimSpace(uid) == "" {
		return contract.Observation{}, output.WrapError(
			"CLUSTER_UNREACHABLE",
			"cannot read cluster identity (kube-system namespace UID) from the live cluster",
			output.ExitDomain, err)
	}

	kubeContext, _ := kubectlValue(kubeconfig, "config", "current-context")

	hostname, gateway, err := observeHarborFrontDoor(kubeconfig)
	if err != nil {
		return contract.Observation{}, err
	}

	caLocator, caFingerprint, err := observeHarborCA(kubeconfig)
	if err != nil {
		return contract.Observation{}, err
	}

	return contract.Observation{
		ClusterIdentity: strings.TrimSpace(uid),
		Context:         strings.TrimSpace(kubeContext),
		KubeconfigPath:  kubeconfig,
		HarborHostname:  hostname,
		HarborGateway:   gateway,
		CAFileLocator:   caLocator,
		CASHA256:        caFingerprint,
	}, nil
}

// observeHarborFrontDoor reads the Harbor Gateway's listener hostname. The
// hostname is a live observation of the Gateway, not a constant.
func observeHarborFrontDoor(kubeconfig string) (hostname, gateway string, err error) {
	const ns, name = "harbor", "harbor-gateway"
	raw, err := runKubectlJSON(kubeconfig, "get", "gateway", name, "-n", ns, "-o", "json")
	if err != nil {
		return "", "", output.WrapError(
			"HARBOR_GATEWAY_UNREACHABLE",
			fmt.Sprintf("cannot read Gateway %s/%s from the live cluster", ns, name),
			output.ExitDomain, err)
	}
	var gw struct {
		Spec struct {
			Listeners []struct {
				Hostname string `json:"hostname"`
			} `json:"listeners"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(raw, &gw); err != nil {
		return "", "", output.WrapError(
			"HARBOR_GATEWAY_PARSE", err.Error(), output.ExitRuntime, err)
	}
	for _, l := range gw.Spec.Listeners {
		if strings.TrimSpace(l.Hostname) != "" {
			return strings.TrimSpace(l.Hostname), ns + "/" + name, nil
		}
	}
	return "", "", output.NewError(
		"HARBOR_HOSTNAME_UNSET",
		fmt.Sprintf("Gateway %s/%s has no listener hostname to observe", ns, name),
		output.ExitDomain)
}

// observeHarborCA reads the Harbor CA certificate from an in-cluster secret and
// returns a secret locator plus the CA fingerprint. The CA bytes themselves are
// never returned to the caller or stored — only their sha256 fingerprint.
func observeHarborCA(kubeconfig string) (locator, fingerprint string, err error) {
	const ns = "harbor"
	// harbor-nginx holds ca.crt for the nodePort/gateway expose types; harbor-tls
	// is the manual TLS secret variant. Try both, first hit wins.
	candidates := []struct{ secret, key string }{
		{"harbor-nginx", "ca.crt"},
		{"harbor-tls", "ca.crt"},
		{"harbor-tls", "tls.crt"},
	}
	var lastErr error
	for _, c := range candidates {
		b64, err := kubectlValue(kubeconfig, "get", "secret", c.secret, "-n", ns,
			"-o", fmt.Sprintf("jsonpath={.data.%s}", strings.ReplaceAll(c.key, ".", `\.`)))
		if err != nil {
			lastErr = err
			continue
		}
		b64 = strings.TrimSpace(b64)
		if b64 == "" {
			continue
		}
		der, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || len(der) == 0 {
			lastErr = err
			continue
		}
		return fmt.Sprintf("secret://%s/%s#%s", ns, c.secret, c.key), contract.Digest(der), nil
	}
	return "", "", output.WrapError(
		"HARBOR_CA_UNREADABLE",
		"cannot read a Harbor CA certificate secret from the live cluster",
		output.ExitDomain, lastErr)
}

// kubectlValue runs kubectl and returns trimmed stdout as a string.
func kubectlValue(kubeconfig string, args ...string) (string, error) {
	allArgs := append([]string{"--kubeconfig", kubeconfig}, args...)
	out, err := exec.Command("kubectl", allArgs...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
