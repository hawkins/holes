# Holes - SSH Tunnel Manager

A command-line interface for managing SSH tunnels, built with the [Huh](https://github.com/charmbracelet/huh) terminal UI library.

## Features

- **Create SSH Tunnels**: Set up local port forwarding through SSH
- **View Tunnels**: List all configured tunnels and their status
- **Close Tunnels**: Terminate active SSH tunnels
- **Interactive UI**: Beautiful terminal forms powered by Huh

## Installation

```bash
go build -o holes
```

## Usage

Run the application:

```bash
./holes
```

The interactive menu will guide you through:

1. **Create new tunnel** - Configure and start a new SSH tunnel
   - Tunnel name (friendly identifier)
   - Local port (port on your machine)
   - Remote host (destination server)
   - Remote port (port on destination)
   - SSH server (tunnel endpoint)
   - SSH user (authentication)

2. **View active tunnels** - Display all configured tunnels with their status

3. **Close a tunnel** - Select and terminate an active tunnel

4. **Exit** - Quit the application

## How It Works

Holes creates SSH tunnels using the `ssh -L` command for local port forwarding. The tunnel configuration is:

```
localhost:LOCAL_PORT -> SSH_SERVER -> REMOTE_HOST:REMOTE_PORT
```

For example, a tunnel configured as:
- Local port: 8080
- Remote host: localhost
- Remote port: 80
- SSH server: example.com
- SSH user: user

Creates the equivalent of:
```bash
ssh -f -N -L 8080:localhost:80 user@example.com
```

## Configuration

Tunnel configurations are stored in `~/.config/holes/tunnels.json`

## Requirements

- Go 1.21 or higher
- SSH client installed on your system
- `lsof` command (for process tracking)

## Project Structure

```
holes/
├── main.go           # CLI interface and menu handlers
├── tunnel/
│   └── tunnel.go     # SSH tunnel management logic
├── go.mod
└── README.md
```

## License

MIT
