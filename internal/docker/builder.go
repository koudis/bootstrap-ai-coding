// Package docker provides Docker client utilities, Dockerfile assembly,
// and container lifecycle helpers.
package docker

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
)

// UserStrategy controls how the Container_User is created inside the image.
type UserStrategy int

const (
	// UserStrategyCreate means no UID/GID conflict exists; the user is created
	// fresh with groupadd + useradd.
	UserStrategyCreate UserStrategy = iota
	// UserStrategyRename means a conflicting user already owns the requested
	// UID/GID; that user is renamed to Container_User with usermod/groupmod.
	UserStrategyRename
)

// DockerfileBuilder assembles a Dockerfile incrementally.
// The full implementation is provided here; agent modules use this type
// via the Agent.Install method.
type DockerfileBuilder struct {
	info          *hostinfo.Info
	lines         []string
	nodeInstalled bool
	gitConfig     string
}

// Username returns the host user's username from the Info struct.
func (b *DockerfileBuilder) Username() string {
	return b.info.Username
}

// HomeDir returns the host user's home directory from the Info struct.
func (b *DockerfileBuilder) HomeDir() string {
	return b.info.HomeDir
}

// MarkNodeInstalled records that a Node.js installation step has already been
// appended to the Dockerfile. Subsequent agents can check IsNodeInstalled()
// to avoid duplicating the Node.js setup.
func (b *DockerfileBuilder) MarkNodeInstalled() {
	b.nodeInstalled = true
}

// IsNodeInstalled reports whether a prior agent has already appended Node.js
// installation steps to this builder.
func (b *DockerfileBuilder) IsNodeInstalled() bool {
	return b.nodeInstalled
}

// NewBaseImageBuilder returns a builder pre-seeded with the shared base layer for
// bac-base:latest. It contains everything that is common across all projects:
//
//   - FROM constants.BaseContainerImage
//   - openssh-server + sudo installation
//   - Container_User creation or rename (controlled by strategy)
//   - passwordless sudo for Container_User
//   - D-Bus + gnome-keyring + profile script
//   - gitconfig injection (if non-empty)
//
// It does NOT include SSH host keys, authorized_keys, sshd_config hardening,
// /run/sshd, or CMD — those belong in the Instance_Image (TL-2).
//
// info carries the runtime-resolved Container_User identity (Req 22).
// strategy controls whether Container_User is created fresh (UserStrategyCreate)
// or an existing conflicting user is renamed (UserStrategyRename).
// conflictingUser is the name of the existing user to rename; it is ignored
// when strategy == UserStrategyCreate.
// gitConfig is the content of the host user's ~/.gitconfig; if empty, the
// injection step is skipped.
func NewBaseImageBuilder(info *hostinfo.Info, strategy UserStrategy, conflictingUser string, gitConfig string) *DockerfileBuilder {
	b := &DockerfileBuilder{info: info, gitConfig: gitConfig}

	// 1. Base image
	b.From(constants.BaseContainerImage)

	// 2. Install openssh-server and sudo
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends openssh-server sudo && rm -rf /var/lib/apt/lists/*")

	// 3. Create or rename the Container_User with matching UID/GID
	switch strategy {
	case UserStrategyRename:
		// A user already owns the requested UID/GID; rename it to Container_User.
		b.Run(fmt.Sprintf(
			"usermod -l %s %s && usermod -d %s -m %s && groupmod -n %s %s",
			info.Username, conflictingUser,
			info.HomeDir, info.Username,
			info.Username, conflictingUser,
		))
	default: // UserStrategyCreate
		b.Run(fmt.Sprintf(
			"groupadd --gid %d %s && useradd --uid %d --gid %d --create-home --shell /bin/bash %s",
			info.GID, info.Username, info.UID, info.GID, info.Username,
		))
	}

	// 4. Passwordless sudo for Container_User
	b.Run(fmt.Sprintf(
		"echo '%s ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers.d/%s && chmod 0440 /etc/sudoers.d/%s",
		info.Username, info.Username, info.Username,
	))

	// 5. Install D-Bus and gnome-keyring for headless credential storage (CC-7).
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends dbus-x11 gnome-keyring libsecret-1-0 && rm -rf /var/lib/apt/lists/*")

	// 6. Install profile.d script that starts D-Bus + gnome-keyring on SSH login.
	keyringScript := `#!/bin/sh\nif [ -z \"$DBUS_SESSION_BUS_ADDRESS\" ]; then\n    eval $(dbus-launch --sh-syntax)\n    export DBUS_SESSION_BUS_ADDRESS\nfi\necho \"\" | gnome-keyring-daemon --unlock --components=secrets 2>/dev/null\n`
	b.Run(fmt.Sprintf("printf '%s' > %s && chmod +x %s",
		keyringScript, constants.KeyringProfileScript, constants.KeyringProfileScript))

	// 7. Inject host user's ~/.gitconfig into the container (Req 24).
	if b.gitConfig != "" {
		encoded := base64.StdEncoding.EncodeToString([]byte(b.gitConfig))
		gitConfigPath := fmt.Sprintf("%s/.gitconfig", info.HomeDir)
		b.Run(fmt.Sprintf(
			"echo %s | base64 -d > %s && chown %s:%s %s && chmod %04o %s",
			encoded, gitConfigPath,
			info.Username, info.Username, gitConfigPath,
			constants.GitConfigPerm, gitConfigPath,
		))
	}

	// NOTE: No SSH host keys, authorized_keys, sshd_config hardening, /run/sshd,
	// or CMD here. Those belong in the Instance_Image (see NewInstanceImageBuilder).

	return b
}

// NewInstanceImageBuilder returns a builder pre-seeded with the instance layer for
// a per-project image (bac-<name>:latest). It starts FROM bac-base:latest and adds
// only the per-project SSH configuration:
//
//   - FROM constants.BaseImageTag (bac-base:latest)
//   - SSH host key injection (hostKeyPriv, hostKeyPub)
//   - authorized_keys setup (publicKey)
//   - sshd_config hardening (no password auth, no root login)
//   - mkdir -p /run/sshd
//
// The caller MUST call Finalize() after this to append the CMD instruction.
//
// info carries the runtime-resolved Container_User identity (Req 22).
// publicKey is the content of the user's SSH public key.
// hostKeyPriv and hostKeyPub are the persisted SSH host key pair contents.
func NewInstanceImageBuilder(info *hostinfo.Info, publicKey, hostKeyPriv, hostKeyPub string) *DockerfileBuilder {
	b := &DockerfileBuilder{info: info}

	// 1. FROM the shared base image
	b.From(constants.BaseImageTag)

	// 2. Inject persisted SSH host key pair (type: constants.SSHHostKeyType)
	privPath := fmt.Sprintf("/etc/ssh/ssh_host_%s_key", constants.SSHHostKeyType)
	pubPath := privPath + ".pub"
	privB64 := base64.StdEncoding.EncodeToString([]byte(hostKeyPriv))
	pubB64 := base64.StdEncoding.EncodeToString([]byte(hostKeyPub))
	b.Run(fmt.Sprintf(
		"echo %s | base64 -d > %s && echo %s | base64 -d > %s && chmod 600 %s && chmod 644 %s",
		privB64, privPath,
		pubB64, pubPath,
		privPath, pubPath,
	))

	// 3. Install SSH public key for Container_User
	pubKeyB64 := base64.StdEncoding.EncodeToString([]byte(publicKey))
	b.Run(fmt.Sprintf(
		"mkdir -p %s/.ssh && echo %s | base64 -d >> %s/.ssh/authorized_keys && chmod 700 %s/.ssh && chmod 600 %s/.ssh/authorized_keys && chown -R %s:%s %s/.ssh",
		info.HomeDir,
		pubKeyB64,
		info.HomeDir,
		info.HomeDir,
		info.HomeDir,
		info.Username, info.Username,
		info.HomeDir,
	))

	// 4. Harden sshd_config
	b.Run("echo 'PasswordAuthentication no' >> /etc/ssh/sshd_config && echo 'PermitRootLogin no' >> /etc/ssh/sshd_config && echo 'PubkeyAuthentication yes' >> /etc/ssh/sshd_config")

	// 5. Ensure sshd runtime dir exists
	b.Run("mkdir -p /run/sshd")

	// NOTE: CMD is intentionally NOT set here. The caller must call Finalize()
	// to append CMD as the very last instruction.

	return b
}



// Finalize appends the CMD instruction that starts sshd in the foreground.
// It must be called after all agent Install() steps and the manifest RUN have
// been appended — CMD must always be the last Dockerfile instruction so that
// agent RUN layers are cached correctly.
func (b *DockerfileBuilder) Finalize() {
	b.lines = append(b.lines, `CMD ["/usr/sbin/sshd", "-D"]`)
}

// From appends a FROM instruction.
func (b *DockerfileBuilder) From(image string) {
	b.lines = append(b.lines, "FROM "+image)
}

// Run appends a RUN instruction.
func (b *DockerfileBuilder) Run(cmd string) {
	b.lines = append(b.lines, "RUN "+cmd)
}

// Env appends an ENV instruction.
func (b *DockerfileBuilder) Env(k, v string) {
	b.lines = append(b.lines, fmt.Sprintf("ENV %s=%s", k, v))
}

// Copy appends a COPY instruction.
func (b *DockerfileBuilder) Copy(src, dst string) {
	b.lines = append(b.lines, fmt.Sprintf("COPY %s %s", src, dst))
}

// Cmd appends a CMD instruction using /bin/sh -c form.
func (b *DockerfileBuilder) Cmd(cmd string) {
	b.lines = append(b.lines, fmt.Sprintf(`CMD ["/bin/sh", "-c", %q]`, cmd))
}

// RunAsUser emits a USER switch, runs the command as the container user,
// then switches back to root for subsequent instructions. This is used by
// agent modules that need to install user-local tools (e.g. uv).
func (b *DockerfileBuilder) RunAsUser(cmd string) {
	b.lines = append(b.lines, fmt.Sprintf("USER %s", b.info.Username))
	b.lines = append(b.lines, "RUN "+cmd)
	b.lines = append(b.lines, "USER root")
}

// Build returns the complete Dockerfile content as a string,
// with each instruction on its own line and a trailing newline.
func (b *DockerfileBuilder) Build() string {
	return strings.Join(b.lines, "\n") + "\n"
}

// Lines returns a copy of the current instruction lines.
// Useful for inspection in tests without triggering a full Build().
func (b *DockerfileBuilder) Lines() []string {
	cp := make([]string, len(b.lines))
	copy(cp, b.lines)
	return cp
}
