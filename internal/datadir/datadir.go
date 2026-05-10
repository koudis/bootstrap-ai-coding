// Package datadir manages the Tool_Data_Dir for each project.
package datadir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/pathutil"
)

// DataDir represents the Tool_Data_Dir for a single project.
type DataDir struct {
	path string
}

// New returns a DataDir for the given container name, creating the directory
// (and all parents) with constants.ToolDataDirPerm if it does not exist.
func New(containerName string) (*DataDir, error) {
	root := pathutil.ExpandHome(constants.ToolDataDirRoot)
	p := filepath.Join(root, containerName)
	if err := os.MkdirAll(p, constants.ToolDataDirPerm); err != nil {
		return nil, fmt.Errorf("creating data dir %s: %w", p, err)
	}
	return &DataDir{path: p}, nil
}

// Path returns the absolute path to this project's data directory.
func (d *DataDir) Path() string { return d.path }

// ReadPort reads the persisted SSH port. Returns 0 if not yet persisted.
func (d *DataDir) ReadPort() (int, error) {
	data, err := os.ReadFile(filepath.Join(d.path, "port"))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading port file: %w", err)
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parsing port file: %w", err)
	}
	return port, nil
}

// WritePort persists the SSH port with constants.ToolDataFilePerm.
func (d *DataDir) WritePort(port int) error {
	p := filepath.Join(d.path, "port")
	if err := os.WriteFile(p, []byte(strconv.Itoa(port)), constants.ToolDataFilePerm); err != nil {
		return fmt.Errorf("writing port file: %w", err)
	}
	return nil
}

// ReadHostKey reads the persisted SSH host key pair.
// Returns ("", "", nil) if not yet generated.
func (d *DataDir) ReadHostKey() (priv, pub string, err error) {
	privPath := filepath.Join(d.path, fmt.Sprintf("ssh_host_%s_key", constants.SSHHostKeyType))
	pubPath := privPath + ".pub"

	privData, err := os.ReadFile(privPath)
	if os.IsNotExist(err) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("reading host private key: %w", err)
	}
	pubData, err := os.ReadFile(pubPath)
	if err != nil {
		return "", "", fmt.Errorf("reading host public key: %w", err)
	}
	return string(privData), string(pubData), nil
}

// WriteHostKey persists the SSH host key pair with constants.ToolDataFilePerm.
func (d *DataDir) WriteHostKey(priv, pub string) error {
	privPath := filepath.Join(d.path, fmt.Sprintf("ssh_host_%s_key", constants.SSHHostKeyType))
	pubPath := privPath + ".pub"

	if err := os.WriteFile(privPath, []byte(priv), constants.ToolDataFilePerm); err != nil {
		return fmt.Errorf("writing host private key: %w", err)
	}
	if err := os.WriteFile(pubPath, []byte(pub), constants.ToolDataFilePerm); err != nil {
		return fmt.Errorf("writing host public key: %w", err)
	}
	return nil
}

// ReadManifest reads the agent manifest. Returns nil if not present.
func (d *DataDir) ReadManifest() ([]string, error) {
	data, err := os.ReadFile(filepath.Join(d.path, "manifest.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading manifest file: %w", err)
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("parsing manifest file: %w", err)
	}
	return ids, nil
}

// WriteManifest persists the agent ID list as JSON with constants.ToolDataFilePerm.
func (d *DataDir) WriteManifest(agentIDs []string) error {
	data, err := json.Marshal(agentIDs)
	if err != nil {
		return fmt.Errorf("marshalling manifest: %w", err)
	}
	p := filepath.Join(d.path, "manifest.json")
	if err := os.WriteFile(p, data, constants.ToolDataFilePerm); err != nil {
		return fmt.Errorf("writing manifest file: %w", err)
	}
	return nil
}

// WriteHostNetworkOff persists the hostNetworkOff boolean as "true" or "false"
// in a file named host_network_off with constants.ToolDataFilePerm.
func (d *DataDir) WriteHostNetworkOff(off bool) error {
	p := filepath.Join(d.path, "host_network_off")
	if err := os.WriteFile(p, []byte(strconv.FormatBool(off)), constants.ToolDataFilePerm); err != nil {
		return fmt.Errorf("writing host_network_off file: %w", err)
	}
	return nil
}

// ReadHostNetworkOff reads the persisted hostNetworkOff value.
// Returns (false, nil) if the file does not exist (default: host network ON).
func (d *DataDir) ReadHostNetworkOff() (bool, error) {
	data, err := os.ReadFile(filepath.Join(d.path, "host_network_off"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading host_network_off file: %w", err)
	}
	val, err := strconv.ParseBool(strings.TrimSpace(string(data)))
	if err != nil {
		return false, fmt.Errorf("parsing host_network_off file: %w", err)
	}
	return val, nil
}

// PurgeRoot removes the entire Tool_Data_Dir root and all its contents.
func PurgeRoot() error {
	return os.RemoveAll(pathutil.ExpandHome(constants.ToolDataDirRoot))
}

// ListContainerNames returns the names of all subdirectories under the
// Tool_Data_Dir root. Each name corresponds to a container that has persisted
// data. Returns nil (not an error) if the root directory does not exist.
func ListContainerNames() ([]string, error) {
	root := pathutil.ExpandHome(constants.ToolDataDirRoot)
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("listing data dir root: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

