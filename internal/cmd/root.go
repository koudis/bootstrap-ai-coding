// Package cmd implements the bootstrap-ai-coding CLI using Cobra.
package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/spf13/cobra"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/credentials"
	"github.com/koudis/bootstrap-ai-coding/internal/datadir"
	dockerpkg "github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/naming"
	"github.com/koudis/bootstrap-ai-coding/internal/portfinder"
	sshpkg "github.com/koudis/bootstrap-ai-coding/internal/ssh"
)

// Mode represents the operating mode of a single CLI invocation.
type Mode int

const (
	ModeStart Mode = iota
	ModeStop
	ModePurge
)

// ResolveMode derives the Mode from the parsed flags.
func ResolveMode(stopAndRemove, purge bool) (Mode, error) {

	switch {
	case stopAndRemove && purge:
		return 0, errors.New("--stop-and-remove and --purge are mutually exclusive")
	case stopAndRemove:
		return ModeStop, nil
	case purge:
		return ModePurge, nil
	default:
		return ModeStart, nil
	}
}

// ValidateStartOnlyFlags returns an error if any start-only flag name is
// provided alongside a non-start mode (ModeStop or ModePurge).
//
// startOnlyFlags is a slice of flag names that were explicitly set by the user
// (i.e. cmd.Flags().Changed(name) == true for each name in the slice).
//
// Validates: CLI-3
func ValidateStartOnlyFlags(mode Mode, changedFlags []string) error {
	if mode == ModeStart {
		return nil
	}
	mf := "--stop-and-remove"
	if mode == ModePurge {
		mf = "--purge"
	}
	startOnly := map[string]bool{
		"agents":               true,
		"port":                 true,
		"ssh-key":              true,
		"rebuild":              true,
		"no-update-known-hosts": true,
		"no-update-ssh-config": true,
		"verbose":              true,
	}
	for _, name := range changedFlags {
		if startOnly[name] {
			return fmt.Errorf("--%s is not valid with %s", name, mf)
		}
	}
	return nil
}

// SessionSummary holds the fields printed to stdout after a successful start.
type SessionSummary struct {
	DataDir       string
	ProjectDir    string
	SSHPort       int
	SSHConnect    string
	EnabledAgents []string
}

// FormatSessionSummary formats a SessionSummary into the five-line output.
func FormatSessionSummary(s SessionSummary) string {
	return fmt.Sprintf(
		"Data directory:  %s\nProject directory: %s\nSSH port:        %d\nSSH connect:     %s\nEnabled agents:  %s\n",
		s.DataDir,
		s.ProjectDir,
		s.SSHPort,
		s.SSHConnect,
		strings.Join(s.EnabledAgents, ", "),
	)
}

// ParseAgentsFlag splits a comma-separated agent ID string, trims whitespace,
// and drops empty strings.
func ParseAgentsFlag(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

var (
	flagAgents             string
	flagPort               int
	flagSSHKey             string
	flagRebuild            bool
	flagStopAndRemove      bool
	flagPurge              bool
	flagNoUpdateKnownHosts bool
	flagNoUpdateSSHConfig  bool
	flagVerbose            bool
)

var rootCmd = &cobra.Command{
	Use:          "bootstrap-ai-coding [project-path]",
	Short:        "Provision an isolated Docker container for AI-assisted coding",
	Version:      constants.Version,
	SilenceUsage: true,
	RunE:         run,
}

// Execute is the entry point called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVar(&flagAgents, "agents", constants.DefaultAgents, "Comma-separated agent IDs to enable")
	rootCmd.Flags().IntVar(&flagPort, "port", 0, "Override SSH port (1024–65535; 0 = auto-select)")
	rootCmd.Flags().StringVar(&flagSSHKey, "ssh-key", "", "Override SSH public key path")
	rootCmd.Flags().BoolVar(&flagRebuild, "rebuild", false, "Force a full container image rebuild")
	rootCmd.Flags().BoolVar(&flagStopAndRemove, "stop-and-remove", false, "Stop and remove the container for the given project")
	rootCmd.Flags().BoolVar(&flagPurge, "purge", false, "Remove all tool data, containers, and images (with confirmation)")
	rootCmd.Flags().BoolVar(&flagNoUpdateKnownHosts, "no-update-known-hosts", false, "Skip automatic ~/.ssh/known_hosts management")
	rootCmd.Flags().BoolVar(&flagNoUpdateSSHConfig, "no-update-ssh-config", false, "Skip automatic ~/.ssh/config management")
	rootCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "Stream Docker build output to stdout in real time")
}

func run(cmd *cobra.Command, args []string) error {
	// Step 1: Flag combination validation (CLI-1 through CLI-6) — always first.
	mode, err := ResolveMode(flagStopAndRemove, flagPurge)
	if err != nil {
		return err
	}

	switch mode {
	case ModeStart, ModeStop:
		if len(args) == 0 {
			_ = cmd.Usage()
			return fmt.Errorf("<project-path> is required")
		}
	case ModePurge:
		if len(args) > 0 {
			return fmt.Errorf("<project-path> must not be provided with --purge")
		}
	}

	if mode == ModeStop || mode == ModePurge {
		if cmd.Flags().Changed("agents") {
			return fmt.Errorf("--agents is not valid with %s", modeFlag(mode))
		}
		if cmd.Flags().Changed("port") {
			return fmt.Errorf("--port is not valid with %s", modeFlag(mode))
		}
		if cmd.Flags().Changed("ssh-key") {
			return fmt.Errorf("--ssh-key is not valid with %s", modeFlag(mode))
		}
		if cmd.Flags().Changed("rebuild") {
			return fmt.Errorf("--rebuild is not valid with %s", modeFlag(mode))
		}
		if cmd.Flags().Changed("no-update-known-hosts") {
			return fmt.Errorf("--no-update-known-hosts is not valid with %s", modeFlag(mode))
		}
		if cmd.Flags().Changed("no-update-ssh-config") {
			return fmt.Errorf("--no-update-ssh-config is not valid with %s", modeFlag(mode))
		}
		if cmd.Flags().Changed("verbose") {
			return fmt.Errorf("--verbose is not valid with %s", modeFlag(mode))
		}
	}

	if mode == ModeStart && cmd.Flags().Changed("port") {
		if flagPort < 1024 || flagPort > 65535 {
			return fmt.Errorf("--port %d is out of range; must be between 1024 and 65535", flagPort)
		}
	}

	var enabledAgents []agent.Agent
	if mode == ModeStart {
		agentIDs := ParseAgentsFlag(flagAgents)
		if len(agentIDs) == 0 {
			return fmt.Errorf("--agents must specify at least one agent ID")
		}
		for _, id := range agentIDs {
			a, err := agent.Lookup(id)
			if err != nil {
				return err
			}
			enabledAgents = append(enabledAgents, a)
		}
	}

	// Step 2: Runtime validation.
	if os.Getuid() == 0 {
		return fmt.Errorf("do not run bootstrap-ai-coding as root; run as a regular user")
	}

	dockerClient, err := dockerpkg.NewClient()
	if err != nil {
		return err
	}
	if err := dockerpkg.CheckVersion(dockerClient); err != nil {
		return err
	}

	// Step 3: Mode dispatch.
	switch mode {
	case ModeStop:
		return runStop(dockerClient, args[0])
	case ModePurge:
		return runPurge(dockerClient)
	default:
		return runStart(dockerClient, args[0], enabledAgents)
	}
}

func modeFlag(m Mode) string {
	if m == ModeStop {
		return "--stop-and-remove"
	}
	return "--purge"
}

func runStop(c *dockerpkg.Client, projectPath string) error {
	existingNames, err := dockerpkg.ListBACContainerNames(context.Background(), c)
	if err != nil {
		return fmt.Errorf("listing existing containers: %w", err)
	}
	containerName, err := naming.ContainerName(projectPath, existingNames)
	if err != nil {
		return fmt.Errorf("deriving container name: %w", err)
	}
	info, err := dockerpkg.InspectContainer(context.Background(), c, containerName)
	if err != nil {
		return err
	}
	if info == nil {
		fmt.Printf("No container found for project %s\n", projectPath)
		return nil
	}
	if err := dockerpkg.StopContainer(context.Background(), c, containerName); err != nil {
		if !strings.Contains(err.Error(), "not running") {
			return err
		}
	}
	if err := dockerpkg.RemoveContainer(context.Background(), c, containerName); err != nil {
		return err
	}
	fmt.Printf("Container %s stopped and removed.\n", containerName)

	// Remove known_hosts entries for this project's SSH port (Req 18.7).
	dd, err := datadir.New(containerName)
	if err == nil {
		if port, err := dd.ReadPort(); err == nil && port != 0 {
			if khErr := sshpkg.RemoveKnownHostsEntries(port); khErr != nil {
				fmt.Fprintf(os.Stderr, "warning: removing known_hosts entries: %v\n", khErr)
			}
		}
	}

	// Remove SSH config entry for this container (Req 19.7).
	if cfgErr := sshpkg.RemoveSSHConfigEntry(containerName); cfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: removing SSH config entry: %v\n", cfgErr)
	}

	return nil
}

func runPurge(c *dockerpkg.Client) error {
	ctx := context.Background()

	containers, err := dockerpkg.ListBACContainers(ctx, c)
	if err != nil {
		return err
	}
	images, err := dockerpkg.ListBACImages(ctx, c)
	if err != nil {
		return err
	}

	// Collect persisted SSH ports BEFORE purging the data dir (Req 18.8).
	// We need these to remove known_hosts entries after the data dir is gone.
	containerNames, err := datadir.ListContainerNames()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: listing data dirs: %v\n", err)
	}
	var persistedPorts []int
	for _, name := range containerNames {
		dd, err := datadir.New(name)
		if err != nil {
			continue
		}
		port, err := dd.ReadPort()
		if err == nil && port != 0 {
			persistedPorts = append(persistedPorts, port)
		}
	}

	fmt.Printf("This will delete:\n")
	fmt.Printf("  %d container(s)\n", len(containers))
	fmt.Printf("  %d image(s)\n", len(images))
	fmt.Printf("  %s\n", expandHome(constants.ToolDataDirRoot))
	fmt.Printf("\nType 'yes' to confirm: ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
		fmt.Println("Purge cancelled.")
		return nil
	}

	for _, ctr := range containers {
		name := ctr.ID
		if len(ctr.Names) > 0 {
			name = ctr.Names[0]
		}
		_ = dockerpkg.StopContainer(ctx, c, ctr.ID)
		if err := dockerpkg.RemoveContainer(ctx, c, ctr.ID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: removing container %s: %v\n", name, err)
		}
	}

	for _, img := range images {
		tag := img.ID
		if len(img.RepoTags) > 0 {
			tag = img.RepoTags[0]
		}
		if _, err := c.ImageRemove(ctx, img.ID, image.RemoveOptions{Force: true}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: removing image %s: %v\n", tag, err)
		}
	}

	if err := datadir.PurgeRoot(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: purging data dir: %v\n", err)
	}

	// Remove known_hosts entries for all persisted SSH ports (Req 18.8).
	for _, port := range persistedPorts {
		if khErr := sshpkg.RemoveKnownHostsEntries(port); khErr != nil {
			fmt.Fprintf(os.Stderr, "warning: removing known_hosts entries for port %d: %v\n", port, khErr)
		}
	}

	// Remove all SSH config entries for bac-managed containers (Req 19.8).
	if cfgErr := sshpkg.RemoveAllBACSSHConfigEntries(); cfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: removing SSH config entries: %v\n", cfgErr)
	}

	fmt.Printf("Purge complete: removed %d container(s), %d image(s), and tool data.\n",
		len(containers), len(images))
	return nil
}

func runStart(c *dockerpkg.Client, projectPath string, enabledAgents []agent.Agent) error {
	ctx := context.Background()

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("resolving project path: %w", err)
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("project path %s does not exist", absPath)
	}

	publicKey, err := sshpkg.DiscoverPublicKey(flagSSHKey)
	if err != nil {
		return err
	}

	existingNames, err := dockerpkg.ListBACContainerNames(ctx, c)
	if err != nil {
		return fmt.Errorf("listing existing containers: %w", err)
	}
	containerName, err := naming.ContainerName(absPath, existingNames)
	if err != nil {
		return fmt.Errorf("deriving container name: %w", err)
	}

	dd, err := datadir.New(containerName)
	if err != nil {
		return err
	}

	sshPort := flagPort
	if sshPort == 0 {
		sshPort, err = dd.ReadPort()
		if err != nil {
			return err
		}
		// Port conflict check for persisted ports (Req 12.5).
		// If the persisted port is in use by something other than our own container,
		// report an error. If our container is already running (it holds the port),
		// this is the normal reconnect path — no error.
		if sshPort != 0 && !portfinder.IsPortFree(sshPort) {
			containerInfo, inspectErr := dockerpkg.InspectContainer(ctx, c, containerName)
			if inspectErr != nil {
				return inspectErr
			}
			if containerInfo == nil || containerInfo.State == nil || !containerInfo.State.Running {
				return fmt.Errorf("persisted SSH port %d is already in use by another process; "+
					"use --port to select a different port", sshPort)
			}
		}
	}
	if sshPort == 0 {
		sshPort, err = portfinder.FindFreePort()
		if err != nil {
			return err
		}
	}
	if err := dd.WritePort(sshPort); err != nil {
		return err
	}

	hostKeyPriv, hostKeyPub, err := dd.ReadHostKey()
	if err != nil {
		return err
	}
	if hostKeyPriv == "" {
		hostKeyPriv, hostKeyPub, err = sshpkg.GenerateHostKeyPair()
		if err != nil {
			return fmt.Errorf("generating SSH host key pair: %w", err)
		}
		if err := dd.WriteHostKey(hostKeyPriv, hostKeyPub); err != nil {
			return err
		}
	}

	type agentCredStatus struct {
		a              agent.Agent
		resolvedPath   string
		hasCredentials bool
	}
	var agentStatuses []agentCredStatus

	for _, a := range enabledAgents {
		resolved := credentials.Resolve(a.CredentialStorePath(), "")
		if err := credentials.EnsureDir(resolved); err != nil {
			return fmt.Errorf("ensuring credential dir for %s: %w", a.ID(), err)
		}
		// If the agent implements CredentialPreparer, let it sync external
		// state into the credential store before we mount it.
		if prep, ok := a.(agent.CredentialPreparer); ok {
			if err := prep.PrepareCredentials(resolved); err != nil {
				fmt.Fprintf(os.Stderr, "warning: preparing credentials for %s: %v\n", a.ID(), err)
			}
		}
		hasCreds, err := a.HasCredentials(resolved)
		if err != nil {
			return fmt.Errorf("checking credentials for %s: %w", a.ID(), err)
		}
		agentStatuses = append(agentStatuses, agentCredStatus{
			a:              a,
			resolvedPath:   resolved,
			hasCredentials: hasCreds,
		})
	}

	imageTag := containerName + ":latest"
	labels := map[string]string{
		"bac.managed":   "true",
		"bac.container": containerName,
	}

	enabledIDs := make([]string, 0, len(enabledAgents))
	for _, a := range enabledAgents {
		enabledIDs = append(enabledIDs, a.ID())
	}

	needBuild := flagRebuild
	if !needBuild {
		imgInfo, _, err := c.ImageInspectWithRaw(ctx, imageTag)
		if err != nil {
			needBuild = true
		} else {
			manifestJSON, ok := imgInfo.Config.Labels["bac.manifest"]
			if !ok {
				needBuild = true
			} else {
				var manifestIDs []string
				if err := json.Unmarshal([]byte(manifestJSON), &manifestIDs); err != nil {
					needBuild = true
				} else if !stringSlicesEqual(manifestIDs, enabledIDs) {
					fmt.Println("Agent config changed — run with --rebuild to update the image.")
					return nil
				}
			}
		}
	}

	if needBuild {
		uid, gid, err := hostUIDGID()
		if err != nil {
			return err
		}

		// Check for UID/GID conflict in base image
		strategy := dockerpkg.UserStrategyCreate
		conflictingUser := ""
		conflictingImageUser, err := dockerpkg.FindConflictingUser(ctx, c, uid, gid)
		if err != nil {
			return fmt.Errorf("checking base image for UID/GID conflicts: %w", err)
		}
		if conflictingImageUser != nil {
			fmt.Printf("User %q (UID %d, GID %d) already exists in the base image.\nRename it to %q? [y/N]: ",
				conflictingImageUser.Username, conflictingImageUser.UID, conflictingImageUser.GID,
				constants.ContainerUser)
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer == "y" {
				strategy = dockerpkg.UserStrategyRename
				conflictingUser = conflictingImageUser.Username
			} else {
				return fmt.Errorf("cannot build image: user %q (UID %d, GID %d) conflicts with the required container user; re-run and confirm the rename",
					conflictingImageUser.Username, conflictingImageUser.UID, conflictingImageUser.GID)
			}
		}

		b := dockerpkg.NewDockerfileBuilder(uid, gid, publicKey, hostKeyPriv, hostKeyPub, strategy, conflictingUser)
		for _, a := range enabledAgents {
			a.Install(b)
		}
		manifestJSON, _ := json.Marshal(enabledIDs)
		b.Run(fmt.Sprintf("echo %q > %s", string(manifestJSON), constants.ManifestFilePath))
		b.Finalize() // CMD must be last — after all agent RUN steps
		labels["bac.manifest"] = string(manifestJSON)

		spec := dockerpkg.ContainerSpec{
			Name:       containerName,
			ImageTag:   imageTag,
			Dockerfile: b.Build(),
			Labels:     labels,
			HostUID:    uid,
			HostGID:    gid,
			NoCache:    flagRebuild,
		}

		fmt.Println("Building image...")
		buildOutput, err := dockerpkg.BuildImage(ctx, c, spec, flagVerbose)
		if err != nil {
			fmt.Fprint(os.Stderr, buildOutput)
			return fmt.Errorf("image build failed: %w", err)
		}
	}

	info, err := dockerpkg.InspectContainer(ctx, c, containerName)
	if err != nil {
		return err
	}
	if info != nil && info.State != nil && info.State.Running {
		if !flagRebuild {
			// Sync known_hosts even when reconnecting to an already-running container (Req 18.1).
			if err := sshpkg.SyncKnownHosts(sshPort, hostKeyPub, flagNoUpdateKnownHosts); err != nil {
				fmt.Fprintf(os.Stderr, "warning: syncing known_hosts: %v\n", err)
			}
			// Sync SSH config entry (Req 19.1).
			if err := sshpkg.SyncSSHConfig(containerName, sshPort, flagNoUpdateSSHConfig); err != nil {
				fmt.Fprintf(os.Stderr, "warning: syncing SSH config: %v\n", err)
			}
			printSessionSummary(dd, absPath, containerName, sshPort, enabledIDs)
			return nil
		}
		// --rebuild: stop the running container so it gets recreated from the new image.
		fmt.Println("Stopping existing container for rebuild...")
		_ = dockerpkg.StopContainer(ctx, c, containerName)
		_ = dockerpkg.RemoveContainer(ctx, c, containerName)
		info = nil
	}

	if info != nil {
		_ = dockerpkg.RemoveContainer(ctx, c, containerName)
	}

	mounts := []dockerpkg.Mount{
		{HostPath: absPath, ContainerPath: constants.WorkspaceMountPath},
	}
	for _, s := range agentStatuses {
		mounts = append(mounts, dockerpkg.Mount{
			HostPath:      s.resolvedPath,
			ContainerPath: s.a.ContainerMountPath(),
		})
	}

	spec := dockerpkg.ContainerSpec{
		Name:     containerName,
		ImageTag: imageTag,
		Mounts:   mounts,
		SSHPort:  sshPort,
		Labels:   labels,
	}

	if _, err := dockerpkg.CreateContainer(ctx, c, spec); err != nil {
		return err
	}
	if err := dockerpkg.StartContainer(ctx, c, containerName); err != nil {
		return err
	}

	if err := dockerpkg.WaitForSSH(ctx, constants.KnownHostsPatterns[1], sshPort, 10*time.Second); err != nil {
		_ = dockerpkg.StopContainer(ctx, c, containerName)
		_ = dockerpkg.RemoveContainer(ctx, c, containerName)
		return fmt.Errorf("container started but SSH did not become ready: %w", err)
	}

	// Sync known_hosts with the container's SSH host key (Req 18.1–18.9).
	if err := sshpkg.SyncKnownHosts(sshPort, hostKeyPub, flagNoUpdateKnownHosts); err != nil {
		fmt.Fprintf(os.Stderr, "warning: syncing known_hosts: %v\n", err)
	}
	// Sync SSH config entry (Req 19.1).
	if err := sshpkg.SyncSSHConfig(containerName, sshPort, flagNoUpdateSSHConfig); err != nil {
		fmt.Fprintf(os.Stderr, "warning: syncing SSH config: %v\n", err)
	}

	for _, s := range agentStatuses {
		if !s.hasCredentials {
			fmt.Printf("Authenticate %s inside the container: run 'claude' and complete the login flow.\n", s.a.ID())
		}
	}

	printSessionSummary(dd, absPath, containerName, sshPort, enabledIDs)
	return nil
}

func printSessionSummary(dd *datadir.DataDir, projectDir string, containerName string, sshPort int, agentIDs []string) {
	summary := SessionSummary{
		DataDir:       dd.Path(),
		ProjectDir:    projectDir,
		SSHPort:       sshPort,
		SSHConnect:    "ssh " + containerName,
		EnabledAgents: agentIDs,
	}
	fmt.Print(FormatSessionSummary(summary))
}

func hostUIDGID() (int, int, error) {
	u, err := user.Current()
	if err != nil {
		return 0, 0, fmt.Errorf("getting current user: %w", err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing UID: %w", err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing GID: %w", err)
	}
	return uid, gid, nil
}

func stringSlicesEqual(a, b []string) bool {
	return StringSlicesEqual(a, b)
}

// StringSlicesEqual reports whether a and b contain the same elements in the same order.
func StringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func expandHome(p string) string {
	return ExpandHome(p)
}

// ExpandHome expands a leading "~/" to the user's home directory.
func ExpandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
