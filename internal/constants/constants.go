// Package constants defines all project-wide constants derived from the
// requirements glossary. Every other package must import from here rather
// than hardcoding these values.
package constants

import "time"

const (
	// BaseContainerImage is the base Docker image for all containers.
	// Corresponds to the Base_Container_Image glossary term.
	BaseContainerImage = "ubuntu:26.04"

	// BaseImageName is the name of the base Docker image used in the
	// two-layer image architecture. The instance image uses FROM BaseImageName:latest.
	BaseImageName = "bac-base"

	// BaseImageTag is the full image reference for the base image (name + ":latest").
	// Used in FROM directives and image inspection calls.
	BaseImageTag = BaseImageName + ":latest"

	// WorkspaceMountPath is the path inside the container where the project is mounted.
	// Corresponds to the Mounted_Volume glossary term.
	WorkspaceMountPath = "/workspace"

	// SSHPortStart is the starting port for SSH port auto-selection.
	// Corresponds to the SSH_Port glossary term.
	SSHPortStart = 2222

	// ContainerSSHPort is the port sshd listens on inside the container (always 22).
	// Derived from the SSH_Port glossary term definition.
	ContainerSSHPort = 22

	// ToolDataDirRoot is the root directory for all tool-generated persistent data.
	// Corresponds to the Tool_Data_Dir glossary term.
	ToolDataDirRoot = "~/.config/bootstrap-ai-coding"

	// ContainerNamePrefix is the prefix for all deterministic container names.
	ContainerNamePrefix = "bac-"

	// ContainerNameParentSep is the separator between <parentdir> and <dirname>
	// in level-2+ container names (e.g. "bac-parent_child").
	// Corresponds to the container naming convention in Req 5.1.
	ContainerNameParentSep = "_"

	// ContainerNameCounterSep is the separator before the numeric counter suffix
	// in level-3+ container names (e.g. "bac-parent_child-2").
	// Corresponds to the container naming convention in Req 5.1.
	ContainerNameCounterSep = "-"

	// ManifestFilePath is the path inside the container image where the agent manifest is stored.
	ManifestFilePath = "/bac-manifest.json"

	// ClaudeCodeAgentName is the stable Agent_ID for the Claude Code agent module.
	// Corresponds to the Agent_ID glossary term for Claude Code (CC-1).
	ClaudeCodeAgentName = "claude-code"

	// AugmentCodeAgentName is the stable Agent_ID for the Augment Code agent module.
	// Corresponds to the Agent_ID glossary term for Augment Code (AC-1).
	AugmentCodeAgentName = "augment-code"

	// BuildResourcesAgentName is the stable Agent_ID for the Build Resources
	// pseudo-agent module that installs common build toolchains and runtimes.
	// Corresponds to the Agent_ID glossary term for Build Resources (BR-1).
	BuildResourcesAgentName = "build-resources"

	// DefaultAgents is the comma-separated list of agent IDs enabled when the
	// --agents flag is omitted. Claude Code, Augment Code, and Build Resources
	// are enabled by default.
	DefaultAgents = ClaudeCodeAgentName + "," + AugmentCodeAgentName + "," + BuildResourcesAgentName

	// SSHHostKeyType is the algorithm used for the container's SSH host key pair.
	// Determines the key file names on disk (ssh_host_<type>_key) and the path
	// injected into the Dockerfile. Not a glossary term — implementation choice
	// satisfying Req 13.1.
	SSHHostKeyType = "ed25519"

	// MinDockerVersion is the minimum required Docker daemon version.
	// Satisfies Req 6.3.
	MinDockerVersion = "20.10"

	// ToolDataDirPerm is the directory permission for Tool_Data_Dir and subdirectories.
	// Satisfies Req 15.2.
	ToolDataDirPerm = 0o700

	// ToolDataFilePerm is the file permission for all files written to Tool_Data_Dir.
	// Satisfies Req 15.3.
	ToolDataFilePerm = 0o600

	// GitConfigPerm is the file permission for the injected .gitconfig inside the container.
	// Read-only for all users. Satisfies Req 24.
	GitConfigPerm = 0o444

	// KnownHostsFile is the path to the SSH client's known_hosts file on the Host.
	// Corresponds to the Known_Hosts_File glossary term.
	// The "~/" prefix is expanded at runtime via os.UserHomeDir().
	KnownHostsFile = "~/.ssh/known_hosts"

	// SSHConfigFile is the path to the SSH client configuration file on the Host.
	// Corresponds to the SSH_Config_File glossary term (Req 19).
	// The "~/" prefix is expanded at runtime via os.UserHomeDir().
	SSHConfigFile = "~/.ssh/config"

	// SSHDirPerm is the permission for the ~/.ssh directory on the Host.
	// Satisfies Req 18.5.
	SSHDirPerm = 0o700

	// KeyringProfileScript is the path to the shell profile script inside the
	// container that starts D-Bus and gnome-keyring-daemon on SSH login.
	// Satisfies CC-7.
	KeyringProfileScript = "/etc/profile.d/dbus-keyring.sh"

	// HostBindIP is the IP address containers bind their SSH port to on the host.
	// Satisfies R7.
	HostBindIP = "127.0.0.1"

	// DefaultRestartPolicy is the Docker restart policy applied to containers
	// by default. "unless-stopped" means the container restarts after a host
	// reboot unless the user explicitly stopped it.
	// Satisfies Req 25.2.
	DefaultRestartPolicy = "unless-stopped"
)

// ImageBuildTimeout is the maximum time allowed for a Docker image build.
// Building installs Node.js and npm packages, so 8 minutes is sufficient
// for a warm cache and bounded — a hung RUN step will be cancelled rather
// than blocking forever.
const ImageBuildTimeout = 8 * time.Minute

// Version is the build version, injected at link time via:
//
//	-ldflags "-X 'github.com/koudis/bootstrap-ai-coding/internal/constants.Version=<tag>'"
//
// Falls back to "dev" when built without ldflags (e.g. `go run .`).
var Version = "dev"

var (
	// PublicKeyDefaultPaths lists the candidate Public_Key file paths on the Host,
	// in order of precedence (highest first). The CLI tries each in turn before
	// falling back to the --ssh-key flag value.
	// Defined by Req 4.1: ~/.ssh/id_ed25519.pub → ~/.ssh/id_rsa.pub → --ssh-key.
	// Declared as a var (not const) because Go does not support slice constants.
	PublicKeyDefaultPaths = []string{
		"~/.ssh/id_ed25519.pub",
		"~/.ssh/id_rsa.pub",
	}

	// KnownHostsPatterns lists the host pattern prefixes written into Known_Hosts_Entry
	// lines for each SSH_Port. Index 0 is the bracket-localhost form, index 1 is the
	// loopback IP form. Both entries are written for every managed SSH_Port.
	// Corresponds to the Known_Hosts_Entry glossary term.
	// Declared as a var (not const) because Go does not support slice constants.
	KnownHostsPatterns = []string{"[localhost]", "127.0.0.1"}
)
