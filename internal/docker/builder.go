// Package docker provides Docker client utilities, Dockerfile assembly,
// and container lifecycle helpers.
package docker

import (
	"fmt"
	"strings"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
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
	lines         []string
	nodeInstalled bool
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

// NewDockerfileBuilder returns a builder pre-seeded with the base layer required
// for every bac container:
//
//   - FROM constants.BaseContainerImage
//   - openssh-server + sudo installation
//   - Container_User creation or rename (controlled by strategy)
//   - passwordless sudo for Container_User
//   - SSH authorized_keys for Container_User
//   - SSH host key injection from Tool_Data_Dir
//   - sshd_config hardening (no password auth, no root login)
//   - mkdir -p /run/sshd
//   - CMD ["/usr/sbin/sshd", "-D"]
//
// uid and gid are the host user's effective UID/GID.
// publicKey is the content of the user's SSH public key.
// hostKeyPriv and hostKeyPub are the persisted SSH host key pair contents
// (key type is always constants.SSHHostKeyType).
// strategy controls whether Container_User is created fresh (UserStrategyCreate)
// or an existing conflicting user is renamed (UserStrategyRename).
// conflictingUser is the name of the existing user to rename; it is ignored
// when strategy == UserStrategyCreate.
func NewDockerfileBuilder(uid, gid int, publicKey, hostKeyPriv, hostKeyPub string, strategy UserStrategy, conflictingUser string) *DockerfileBuilder {
	b := &DockerfileBuilder{}

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
			constants.ContainerUser, conflictingUser,
			constants.ContainerUserHome, constants.ContainerUser,
			constants.ContainerUser, conflictingUser,
		))
	default: // UserStrategyCreate
		b.Run(fmt.Sprintf(
			"groupadd --gid %d %s && useradd --uid %d --gid %d --create-home --shell /bin/bash %s",
			gid, constants.ContainerUser, uid, gid, constants.ContainerUser,
		))
	}

	// 4. Passwordless sudo for Container_User
	b.Run(fmt.Sprintf(
		"echo '%s ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers.d/%s && chmod 0440 /etc/sudoers.d/%s",
		constants.ContainerUser, constants.ContainerUser, constants.ContainerUser,
	))

	// 5. Install SSH public key for Container_User
	// %q is used to safely quote the key content so special characters are escaped.
	b.Run(fmt.Sprintf(
		"mkdir -p %s/.ssh && echo %s >> %s/.ssh/authorized_keys && chmod 700 %s/.ssh && chmod 600 %s/.ssh/authorized_keys && chown -R %s:%s %s/.ssh",
		constants.ContainerUserHome,
		fmt.Sprintf("%q", publicKey),
		constants.ContainerUserHome,
		constants.ContainerUserHome,
		constants.ContainerUserHome,
		constants.ContainerUser, constants.ContainerUser,
		constants.ContainerUserHome,
	))

	// 6. Inject persisted SSH host key pair (type: constants.SSHHostKeyType)
	privPath := fmt.Sprintf("/etc/ssh/ssh_host_%s_key", constants.SSHHostKeyType)
	pubPath := privPath + ".pub"
	b.Run(fmt.Sprintf(
		"echo %s > %s && echo %s > %s && chmod 600 %s && chmod 644 %s",
		fmt.Sprintf("%q", hostKeyPriv), privPath,
		fmt.Sprintf("%q", hostKeyPub), pubPath,
		privPath, pubPath,
	))

	// 7. Harden sshd_config
	b.Run("echo 'PasswordAuthentication no' >> /etc/ssh/sshd_config && echo 'PermitRootLogin no' >> /etc/ssh/sshd_config && echo 'PubkeyAuthentication yes' >> /etc/ssh/sshd_config")

	// 8. Ensure sshd runtime dir exists
	b.Run("mkdir -p /run/sshd")

	// NOTE: CMD is intentionally NOT set here. The caller (cmd/root.go) must
	// append agent Install() steps and the manifest RUN, then call Finalize()
	// to append the CMD as the very last instruction. This ensures all RUN
	// steps are ordered before CMD so Docker's layer cache is not busted by
	// agent install steps appearing after CMD.

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
