package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
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
	configPath     string
	lastTunnelPath string
	tunnels        map[string]*Tunnel
	mu             sync.RWMutex
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
	lastTunnelPath := filepath.Join(configDir, "last_tunnel.json")

	m := &Manager{
		configPath:     configPath,
		lastTunnelPath: lastTunnelPath,
		tunnels:        make(map[string]*Tunnel),
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

// saveLastTunnel saves the last created tunnel configuration
func (m *Manager) saveLastTunnel(t *Tunnel) error {
	// Create a copy without the PID for use as a template
	template := &Tunnel{
		Name:       t.Name,
		LocalPort:  t.LocalPort,
		RemoteHost: t.RemoteHost,
		RemotePort: t.RemotePort,
		SSHServer:  t.SSHServer,
		SSHUser:    t.SSHUser,
	}

	data, err := json.Marshal(template)
	if err != nil {
		return fmt.Errorf("failed to marshal last tunnel: %w", err)
	}

	return os.WriteFile(m.lastTunnelPath, data, 0644)
}

// LoadLastTunnel loads the last created tunnel configuration if it exists
func (m *Manager) LoadLastTunnel() (*Tunnel, error) {
	data, err := os.ReadFile(m.lastTunnelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No last tunnel is not an error
		}
		return nil, fmt.Errorf("failed to read last tunnel: %w", err)
	}

	var t Tunnel
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("failed to unmarshal last tunnel: %w", err)
	}

	return &t, nil
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

	if err := m.save(); err != nil {
		return err
	}

	// Save as last tunnel for future placeholders
	return m.saveLastTunnel(t)
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
	if err != nil {
		return false
	}

	// Double-check that the process is actually SSH
	// This prevents false positives from PID reuse
	if !isSSHProcess(t.PID) {
		return false
	}

	// Verify the process is listening on the expected port
	// This ensures it's our specific tunnel, not just any SSH process
	return isProcessListeningOnPort(t.PID, t.LocalPort)
}

// isProcessListeningOnPort checks if a process is listening on a specific port
func isProcessListeningOnPort(pid, port int) bool {
	// Use lsof to check if this specific PID is listening on the port
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf("TCP:%d", port), "-sTCP:LISTEN", "-a", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	pidStr := strings.TrimSpace(string(output))
	foundPID, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}

	return foundPID == pid
}

// findSSHProcess attempts to find the PID of an SSH process for a given local port
// Uses retry logic with exponential backoff to handle race conditions
func findSSHProcess(localPort int) (int, error) {
	maxRetries := 10
	initialDelay := 50 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 50ms, 100ms, 200ms, 400ms, etc.
			delay := initialDelay * time.Duration(1<<uint(attempt-1))
			if delay > 2*time.Second {
				delay = 2 * time.Second
			}
			time.Sleep(delay)
		}

		pid, err := findSSHProcessOnce(localPort)
		if err == nil {
			return pid, nil
		}

		// If it's not a "not found" error, return immediately
		if !strings.Contains(err.Error(), "no process found") &&
		   !strings.Contains(err.Error(), "exit status 1") {
			return 0, err
		}
	}

	return 0, fmt.Errorf("failed to find SSH process on port %d after %d attempts", localPort, maxRetries)
}

// findSSHProcessOnce makes a single attempt to find the SSH process
func findSSHProcessOnce(localPort int) (int, error) {
	// Use lsof with -iTCP to catch both IPv4 and IPv6
	// -t: terse output (PIDs only)
	// -iTCP:PORT: look for TCP connections on this port
	// -sTCP:LISTEN: only show listening sockets
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf("TCP:%d", localPort), "-sTCP:LISTEN")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("no process found on port %d: %w", localPort, err)
	}

	pidStr := strings.TrimSpace(string(output))
	if len(pidStr) == 0 {
		return 0, fmt.Errorf("no process found on port %d", localPort)
	}

	// If multiple PIDs, take the first line (most likely the SSH process)
	lines := strings.Split(pidStr, "\n")
	firstPID := strings.TrimSpace(lines[0])

	pid, err := strconv.Atoi(firstPID)
	if err != nil {
		return 0, fmt.Errorf("failed to parse PID '%s': %w", firstPID, err)
	}

	// Verify it's actually an SSH process
	if !isSSHProcess(pid) {
		return 0, fmt.Errorf("process %d on port %d is not an SSH process", pid, localPort)
	}

	return pid, nil
}

// isSSHProcess checks if a given PID is an SSH process
func isSSHProcess(pid int) bool {
	// Use ps to get the command name
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	cmdName := strings.TrimSpace(string(output))
	return cmdName == "ssh"
}
