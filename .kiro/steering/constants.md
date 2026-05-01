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
| `ContainerNameHashLen` | `12` | container naming convention |
| `ManifestFilePath` | `"/bac-manifest.json"` | manifest file inside image |
| `DefaultAgent` | `"claude-code"` | default `Enabled_Agents` (Req 7.5) |
| `SSHHostKeyType` | `"ed25519"` | SSH host key algorithm |
| `MinDockerVersion` | `"20.10"` | minimum Docker version (Req 6.3) |
| `ToolDataDirPerm` | `0o700` | Tool_Data_Dir permissions (Req 15.2) |
| `ToolDataFilePerm` | `0o600` | Tool_Data_Dir file permissions (Req 15.3) |

## Import Pattern

```go
import "github.com/user/bootstrap-ai-coding/constants"

// Use like:
b.From(constants.BaseContainerImage)
fmt.Sprintf("%s%x", constants.ContainerNamePrefix, hash[:constants.ContainerNameHashLen/2])
os.MkdirAll(path, constants.ToolDataDirPerm)
for port := constants.SSHPortStart; port < 65535; port++ { ... }
```

## Who Can Import `constants`

All packages — including agent modules — may import `constants`. It has no dependencies of its own (pure data), so it creates no import cycles.

## Changing a Constant

If a glossary term changes (e.g. the base image is updated to a new Ubuntu LTS), update **only** `constants/constants.go`. All other packages pick up the change automatically.
