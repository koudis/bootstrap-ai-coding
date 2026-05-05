# Constants

All project-wide constants are defined in `constants/constants.go`. Every value that originates from the requirements glossary lives here. No other package may hardcode these values.

## The Rule

> **If a value appears in the requirements glossary, it must be defined in `constants/` and referenced via `constants.<Name>` everywhere else.**

This means:
- No `"ubuntu:26.04"` string literals outside `constants/`
- No `"dev"` username literals outside `constants/`
- No `2222` port literals outside `constants/`
- No `0o700` / `0o600` permission literals outside `constants/`
- etc.

## Constants Reference

| Constant | Value | Glossary Term |
|---|---|---|
| `BaseContainerImage` | `"ubuntu:26.04"` | `Base_Container_Image` |
| `ContainerUser` | `"dev"` | `Container_User` (username) |
| `ContainerUserHome` | `"/home/" + ContainerUser` | `Container_User_Home` |
| `WorkspaceMountPath` | `"/workspace"` | `Mounted_Volume` (container path) |
| `SSHPortStart` | `2222` | `SSH_Port` (starting value) |
| `ContainerSSHPort` | `22` | SSH port inside the container |
| `ToolDataDirRoot` | `"~/.config/bootstrap-ai-coding"` | `Tool_Data_Dir` (root) |
| `ContainerNamePrefix` | `"bac-"` | container naming convention |
| `ContainerNameParentSep` | `"_"` | separator between parentdir and dirname in container names (Req 5.1) |
| `ContainerNameCounterSep` | `"-"` | separator before numeric counter suffix in container names (Req 5.1) |
| `ManifestFilePath` | `"/bac-manifest.json"` | manifest file inside image |
| `ClaudeCodeAgentName` | `"claude-code"` | Agent_ID for Claude Code (CC-1) |
| `AugmentCodeAgentName` | `"augment-code"` | Agent_ID for Augment Code (AC-1) |
| `DefaultAgents` | `"claude-code,augment-code"` | default `Enabled_Agents` (Req 7.5) |
| `SSHHostKeyType` | `"ed25519"` | SSH host key algorithm |
| `MinDockerVersion` | `"20.10"` | minimum Docker version (Req 6.3) |
| `ToolDataDirPerm` | `0o700` | Tool_Data_Dir permissions (Req 15.2) |
| `ToolDataFilePerm` | `0o600` | Tool_Data_Dir file permissions (Req 15.3) |
| `KnownHostsFile` | `"~/.ssh/known_hosts"` | Known_Hosts_File (Req 18) |
| `SSHConfigFile` | `"~/.ssh/config"` | SSH_Config_File (Req 19) |
| `SSHDirPerm` | `0o700` | ~/.ssh directory permissions (Req 18.5) |
| `VSCodeServerPath` | `ContainerUserHome + "/.vscode-server"` | VSCode_Server_Volume mount path (Req 22.1) |
| `VSCodeServerVolumeSuffix` | `"-vscode-server"` | VSCode_Server_Volume name suffix (Req 22.2) |
| `ImageBuildTimeout` | `8 * time.Minute` | Image_Build_Timeout (Req 14.7) |

### Variables (not const — Go does not support slice/map constants)

| Variable | Value | Glossary Term |
|---|---|---|
| `PublicKeyDefaultPaths` | `["~/.ssh/id_ed25519.pub", "~/.ssh/id_rsa.pub"]` | Public_Key discovery order (Req 4.1) |
| `KnownHostsPatterns` | `["[localhost]", "127.0.0.1"]` | Known_Hosts_Entry host patterns (Req 18) |
| `Version` | `"dev"` (overridden via ldflags) | build version |

## Import Pattern

```go
import "github.com/koudis/bootstrap-ai-coding/internal/constants"

// Use like:
b.From(constants.BaseContainerImage)
os.MkdirAll(path, constants.ToolDataDirPerm)
for port := constants.SSHPortStart; port < 65535; port++ { ... }
```

## Who Can Import `constants`

All packages — including agent modules — may import `constants`. It has no dependencies of its own (pure data), so it creates no import cycles.

## Changing a Constant

If a glossary term changes (e.g. the base image is updated to a new Ubuntu LTS), update **only** `constants/constants.go`. All other packages pick up the change automatically.
