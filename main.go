package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/hawkins/holes/tunnel"
)

func main() {
	manager, err := tunnel.NewManager()
	if err != nil {
		fmt.Printf("Failed to initialize tunnel manager: %v\n", err)
		os.Exit(1)
	}

	var action string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Holes - SSH Tunnel Manager").
				Description("What would you like to do?").
				Options(
					huh.NewOption("Create new tunnel", "create"),
					huh.NewOption("View active tunnels", "view"),
					huh.NewOption("Close a tunnel", "close"),
					huh.NewOption("Exit", "exit"),
				).
				Value(&action),
		),
	)

	err = form.Run()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	switch action {
	case "create":
		createTunnel(manager)
	case "view":
		viewTunnels(manager)
	case "close":
		closeTunnel(manager)
	case "exit":
		fmt.Println("Goodbye!")
	}
}

func createTunnel(manager *tunnel.Manager) {
	var (
		name       string
		localPort  string
		remoteHost string
		remotePort string
		sshServer  string
		sshUser    string
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Tunnel Name").
				Description("A friendly name for this tunnel").
				Value(&name).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("name cannot be empty")
					}
					return nil
				}),
			huh.NewInput().
				Title("Local Port").
				Description("Port on your local machine").
				Value(&localPort).
				Placeholder("8080").
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("must be a valid port number")
					}
					return nil
				}),
			huh.NewInput().
				Title("Remote Host").
				Description("Destination host (e.g., localhost, 192.168.1.1)").
				Value(&remoteHost).
				Placeholder("localhost").
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("remote host cannot be empty")
					}
					return nil
				}),
			huh.NewInput().
				Title("Remote Port").
				Description("Port on the remote host").
				Value(&remotePort).
				Placeholder("80").
				Validate(func(s string) error {
					if _, err := strconv.Atoi(s); err != nil {
						return fmt.Errorf("must be a valid port number")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("SSH Server").
				Description("SSH server to tunnel through").
				Value(&sshServer).
				Placeholder("ssh.example.com:22").
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("SSH server cannot be empty")
					}
					return nil
				}),
			huh.NewInput().
				Title("SSH User").
				Description("Username for SSH connection").
				Value(&sshUser).
				Placeholder("user").
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("SSH user cannot be empty")
					}
					return nil
				}),
		),
	)

	err := form.Run()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Convert string ports to integers
	lPort, _ := strconv.Atoi(localPort)
	rPort, _ := strconv.Atoi(remotePort)

	// Create tunnel configuration
	t := &tunnel.Tunnel{
		Name:       name,
		LocalPort:  lPort,
		RemoteHost: remoteHost,
		RemotePort: rPort,
		SSHServer:  sshServer,
		SSHUser:    sshUser,
	}

	// Create the tunnel
	if err := manager.Create(t); err != nil {
		fmt.Printf("\nFailed to create tunnel: %v\n", err)
		return
	}

	fmt.Printf("\n✓ Tunnel '%s' created successfully!\n", name)
	fmt.Printf("  Local: localhost:%d\n", lPort)
	fmt.Printf("  Remote: %s:%d\n", remoteHost, rPort)
	fmt.Printf("  Via: %s@%s\n", sshUser, sshServer)
}

func viewTunnels(manager *tunnel.Manager) {
	tunnels := manager.List()

	if len(tunnels) == 0 {
		fmt.Println("\nNo tunnels configured.")
		return
	}

	fmt.Println("\nConfigured Tunnels:")
	fmt.Println("==================")

	for _, t := range tunnels {
		status := "inactive"
		if manager.IsActive(t.Name) {
			status = "active"
		}

		fmt.Printf("\n%s [%s]\n", t.Name, status)
		fmt.Printf("  Local:  localhost:%d\n", t.LocalPort)
		fmt.Printf("  Remote: %s:%d\n", t.RemoteHost, t.RemotePort)
		fmt.Printf("  Via:    %s@%s\n", t.SSHUser, t.SSHServer)
		if t.PID > 0 {
			fmt.Printf("  PID:    %d\n", t.PID)
		}
	}
	fmt.Println()
}

func closeTunnel(manager *tunnel.Manager) {
	tunnels := manager.List()

	if len(tunnels) == 0 {
		fmt.Println("\nNo tunnels to close.")
		return
	}

	// Build options for tunnel selection
	options := make([]huh.Option[string], 0, len(tunnels))
	for _, t := range tunnels {
		status := "inactive"
		if manager.IsActive(t.Name) {
			status = "active"
		}
		label := fmt.Sprintf("%s [%s] - localhost:%d -> %s:%d",
			t.Name, status, t.LocalPort, t.RemoteHost, t.RemotePort)
		options = append(options, huh.NewOption(label, t.Name))
	}

	var selectedTunnel string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select tunnel to close").
				Options(options...).
				Value(&selectedTunnel),
		),
	)

	err := form.Run()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Close the selected tunnel
	if err := manager.Close(selectedTunnel); err != nil {
		fmt.Printf("\nFailed to close tunnel: %v\n", err)
		return
	}

	fmt.Printf("\n✓ Tunnel '%s' closed successfully!\n", selectedTunnel)
}
