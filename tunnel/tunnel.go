package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
)

// Tunnel represents an SSH tunnel configuration
type Tunnel struct {
	Name       string `json:"name"`
	LocalPort  int    `json:"local_port"`
	RemoteHost string `json:"remote_host"`
	RemotePort int    `json:"remote_port"`
	SSHServer  string `json:"ssh_server"`
	SSHUser    string `json:"ssh_user"`
	PID        int    `json:"pid,omitempty"`
}

// Manager handles tunnel operations
type Manager struct {
	configPath string
	tunnels    map[string]*Tunnel
	mu         sync.RWMutex
}

// NewManager creates a new tunnel manager
func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "holes")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "tunnels.json")

	m := &Manager{
		configPath: configPath,
		tunnels:    make(map[string]*Tunnel),
	}

	// Load existing tunnels
	if err := m.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load tunnels: %w", err)
	}

	return m, nil
}

// load reads tunnels from the config file
func (m *Manager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &m.tunnels)
}

// save writes tunnels to the config file
func (m *Manager) save() error {
	data, err := json.Marshal(m.tunnels)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnels: %w", err)
	}

	return os.WriteFile(m.configPath, data, 0644)
}

// Create creates and starts a new SSH tunnel
func (m *Manager) Create(t *Tunnel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tunnels[t.Name]; exists {
		return fmt.Errorf("tunnel '%s' already exists", t.Name)
	}

	// Build SSH command
	// ssh -f -N -L localPort:remoteHost:remotePort user@sshServer
	localBind := fmt.Sprintf("%d:%s:%d", t.LocalPort, t.RemoteHost, t.RemotePort)

	cmd := exec.Command("ssh",
		"-f",           // Run in background
		"-N",           // Don't execute remote command
		"-L", localBind, // Local port forwarding
		fmt.Sprintf("%s@%s", t.SSHUser, t.SSHServer),
	)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start SSH tunnel: %w", err)
	}

	// Wait for the process to actually background
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("SSH tunnel failed: %w", err)
	}

	// Find the SSH process PID
	// This is a simplified approach - in production you'd want better PID tracking
	pid, err := findSSHProcess(t.LocalPort)
	if err != nil {
		return fmt.Errorf("tunnel started but PID not found: %w", err)
	}

	t.PID = pid
	m.tunnels[t.Name] = t

	return m.save()
}

// List returns all configured tunnels
func (m *Manager) List() []*Tunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tunnels := make([]*Tunnel, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		tunnels = append(tunnels, t)
	}

	return tunnels
}

// Get retrieves a tunnel by name
func (m *Manager) Get(name string) (*Tunnel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, exists := m.tunnels[name]
	if !exists {
		return nil, fmt.Errorf("tunnel '%s' not found", name)
	}

	return t, nil
}

// Close closes an active tunnel
func (m *Manager) Close(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tunnels[name]
	if !exists {
		return fmt.Errorf("tunnel '%s' not found", name)
	}

	if t.PID > 0 {
		// Kill the SSH process
		process, err := os.FindProcess(t.PID)
		if err != nil {
			return fmt.Errorf("failed to find process %d: %w", t.PID, err)
		}

		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process %d: %w", t.PID, err)
		}
	}

	delete(m.tunnels, name)
	return m.save()
}

// IsActive checks if a tunnel is currently active
func (m *Manager) IsActive(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, exists := m.tunnels[name]
	if !exists || t.PID == 0 {
		return false
	}

	// Check if process is still running
	process, err := os.FindProcess(t.PID)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process exists
	err = process.Signal(os.Signal(nil))
	return err == nil
}

// findSSHProcess attempts to find the PID of an SSH process for a given local port
func findSSHProcess(localPort int) (int, error) {
	// Use lsof to find the process listening on the local port
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf("tcp:%d", localPort))
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to find process on port %d: %w", localPort, err)
	}

	pidStr := string(output)
	if len(pidStr) == 0 {
		return 0, fmt.Errorf("no process found on port %d", localPort)
	}

	// Parse the first PID (trim whitespace and newlines)
	pidStr = pidStr[:len(pidStr)-1]
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse PID: %w", err)
	}

	return pid, nil
}
