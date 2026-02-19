# mautrix-simplex

A Matrix-SimpleX puppeting bridge built on [mautrix-go](https://github.com/mautrix/go) bridgev2.

## Features

- Text messages with formatting (bold, italic, strikethrough, code)
- Files, images, video, and audio
- Reactions (SimpleX supports 8 emoji: `👍👎😀😂😢❤🚀✅`)
- Message edits and deletes
- Group chats and DMs
- Reply quoting
- Contact request auto-accept
- Backfill of recent messages on login
- Beeper support (hungryserv/websocket mode)

## Requirements

- [simplex-chat](https://github.com/simplex-chat/simplex-chat) binary (v6.x+)
- Go 1.25+ (to build from source)
- A Matrix homeserver that supports application services (Synapse, Conduit, etc.)

## Building

```bash
cd mautrix-simplex
go build -o mautrix-simplex ./cmd/mautrix-simplex/
```

With version info:

```bash
go build -ldflags "-X main.Tag=v0.1.0 -X main.Commit=$(git rev-parse HEAD) -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o mautrix-simplex ./cmd/mautrix-simplex/
```

## Setup

### 1. Start simplex-chat

The bridge requires a running `simplex-chat` instance with WebSocket API enabled:

```bash
simplex-chat -p 5225 -d /path/to/simplex-data --files-folder /path/to/files --temp-folder /path/to/files/tmp
```

Both `--files-folder` and `--temp-folder` **must be on the same filesystem** to avoid cross-device rename errors. The `--temp-folder` should be a subdirectory of `--files-folder`.

### 2. Generate config

```bash
./mautrix-simplex -g
# Edit config.yaml with your homeserver details
```

### 3. Register with your homeserver

```bash
# Copy the generated registration file to your homeserver
cp registration.yaml /etc/synapse/mautrix-simplex-registration.yaml
# Add it to your homeserver's app_service_config_files and restart
```

### 4. Start the bridge

```bash
./mautrix-simplex
```

### 5. Log in

Use the `login` command in your management room or via the provisioning API.

## Login Modes

### WebSocket (external simplex-chat)

Connect to an already-running simplex-chat instance by providing its WebSocket URL (e.g. `ws://localhost:5225`). Use this when you manage the simplex-chat process separately.

### Managed (bridge-spawned)

Provide a SimpleX database directory path and the bridge will spawn and manage a simplex-chat process automatically. The `simplex_binary` config option controls which binary is used (defaults to `simplex-chat` in `$PATH`).

## Configuration

The network-specific config section supports:

| Key | Description | Default |
|-----|-------------|---------|
| `displayname_template` | Go template for ghost display names | `{{.DisplayName}} (SimpleX)` |
| `simplex_binary` | Path to simplex-chat binary (for managed mode) | `simplex-chat` |
| `files_folder` | Folder where simplex-chat stores files (must match `--files-folder`) | `~/Downloads` |

## Docker

### Build

```bash
cd mautrix-simplex
docker build -t mautrix-simplex .
```

### Run

See the included `docker-compose.yaml` for a full example with simplex-chat sidecar.

```bash
docker compose up -d
```

The compose file runs simplex-chat alongside the bridge with a shared volume for file transfers.

## NixOS

A NixOS module is provided via the flake:

```nix
{
  inputs.mautrix-simplex.url = "github:tricked-dev/mautrix-simplex";

  # In your NixOS configuration:
  imports = [ inputs.mautrix-simplex.nixosModules.default ];

  services.mautrix-simplex = {
    # Enable the bridge and companion simplex-chat service.
    enable = true;

    # Matrix user ID of the bridge owner. Only this user gets admin access,
    # everyone else is blocked. Set to null to configure permissions manually.
    owner = "@alice:example.com";

    # Override the mautrix-simplex package (defaults to the flake's package).
    # package = pkgs.mautrix-simplex;

    # The simplex-chat package for the companion service.
    # Defaults to the bundled binary from the flake.
    # simplexChatPackage = inputs.mautrix-simplex.packages.${pkgs.system}.simplex-chat;

    # Base directory for all state (database, simplex-chat data, files).
    # Defaults to /var/lib/mautrix-simplex.
    dataDir = "/var/lib/selfhosted/matrix/mautrix-simplex";

    # Port for the companion simplex-chat WebSocket API (default: 5225).
    simplexChatPort = 5225;

    # Directory for simplex-chat file storage (default: ${dataDir}/files).
    # Must be on the same filesystem as the temp folder.
    filesFolder = "/var/lib/selfhosted/matrix/mautrix-simplex/files";

    # Whether to automatically register the appservice with Synapse.
    # Defaults to true when services.matrix-synapse is enabled.
    registerToSynapse = true;

    # Extra systemd units to wait for before starting the bridge.
    # Defaults to [ matrix-synapse.service ] when Synapse is enabled.
    # serviceDependencies = [];

    # Bridge configuration, converted to YAML. All mautrix-go bridgev2 config
    # options are available here. The module sets sensible defaults for most fields.
    settings = {
      # Homeserver connection.
      homeserver = {
        address = "http://localhost:8008";
        domain = "example.com";
        # software = "standard";   # or "hungry" for hungryserv
        # websocket = false;       # set true for Beeper
      };

      # Appservice registration (tokens are required).
      appservice = {
        # database.type = "sqlite3-fk-wal";
        # database.uri = "${dataDir}/mautrix-simplex.db"; # auto-set from dataDir
        # id = "simplex";
        # bot.username = "simplexbot";
        # bot.displayname = "SimpleX Bridge Bot";
        as_token = ""; # fill in after registration
        hs_token = ""; # fill in after registration
      };

      bridge = {
        # command_prefix = "!simplex";

        # When owner is set, permissions are auto-configured. Otherwise set manually:
        # permissions = {
        #   "*" = "block";
        #   "@alice:example.com" = "admin";
        #   "@bob:example.com" = "user";    # can use but not admin
        #   "example.com" = "relay";         # whole domain gets relay access
        # };
      };

      # Network-specific config for the SimpleX connector.
      network = {
        # Go template for ghost display names.
        # displayname_template = "{{.DisplayName}} (SimpleX)";

        # Path to simplex-chat binary (auto-set from simplexChatPackage).
        # simplex_binary = "/path/to/simplex-chat";

        # File folder (auto-set from filesFolder option).
        # files_folder = "/path/to/files";
      };
    };
  };
}
```

### Options reference

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enable` | bool | `false` | Enable the bridge and companion simplex-chat service |
| `owner` | string or null | `null` | Matrix ID of sole user; blocks everyone else when set |
| `package` | package | flake default | The mautrix-simplex package |
| `simplexChatPackage` | package | bundled binary | The simplex-chat package for the companion service |
| `dataDir` | string | `/var/lib/mautrix-simplex` | Base directory for all state |
| `simplexChatPort` | port | `5225` | WebSocket API port for simplex-chat |
| `filesFolder` | string | `${dataDir}/files` | simplex-chat file storage directory |
| `registerToSynapse` | bool | auto | Auto-register appservice with Synapse |
| `serviceDependencies` | list of strings | auto | Systemd units to wait for |
| `settings` | attrset | _(see above)_ | Full bridge config (converted to YAML) |

The module automatically:
- Runs a companion simplex-chat systemd service (localhost-only WebSocket)
- Manages state directories and file permissions
- Uses DynamicUser when dataDir is under `/var/lib/`, static user otherwise
- Generates the appservice registration file

## Beeper

The bridge has `BeeperBridgeType: "simplex"` built in and supports Beeper's websocket mode natively. You need [bbctl](https://github.com/beeper/bridge-manager) (Beeper bridge manager) to register and connect the bridge.

Self-hosted bridges on Beeper are **free** and don't count against account limits.

### Manual setup

1. Install bbctl and log in:

```bash
bbctl login
```

2. Generate a bridgev2 config for the bridge:

```bash
bbctl config --type bridgev2 sh-simplex
```

3. Edit the generated config to add network settings (simplex_binary, files_folder, etc.) and ensure websocket mode is enabled:

```yaml
homeserver:
  websocket: true
  address: https://matrix.beeper.com
network:
  simplex_binary: simplex-chat
  files_folder: /path/to/files
```

4. Start simplex-chat alongside the bridge:

```bash
simplex-chat -p 5225 -d /path/to/simplex-data --files-folder /path/to/files --temp-folder /path/to/files/tmp
```

5. Run the bridge with the generated config:

```bash
./mautrix-simplex -c config.yaml
```

6. Log in via the bridge bot DM in Beeper using the `login` command.

### NixOS + Beeper

The flake includes `bbctl` as a package. Here's a full NixOS config for running mautrix-simplex with Beeper:

```nix
{
  inputs.mautrix-simplex.url = "github:tricked-dev/mautrix-simplex";

  imports = [ inputs.mautrix-simplex.nixosModules.default ];

  # bbctl available as a package
  environment.systemPackages = [
    inputs.mautrix-simplex.packages.${system}.bbctl
  ];

  services.mautrix-simplex = {
    enable = true;
    owner = "@you:beeper.local"; # restrict to your Beeper account
    dataDir = "/var/lib/selfhosted/matrix/mautrix-simplex"; # optional, custom base path

    settings = {
      homeserver = {
        websocket = true;
        address = "https://matrix.beeper.com";
        domain = "beeper.local";
      };
    };
  };
}
```

Before starting the services, run `bbctl login` and `bbctl config --type bridgev2 sh-simplex` to generate the registration and tokens, then add the `as_token` and `hs_token` to your settings.

### Setup steps on NixOS

```bash
# 1. Install bbctl (available via the flake)
nix shell github:tricked-dev/mautrix-simplex#bbctl

# 2. Authenticate with Beeper
bbctl login

# 3. Register the bridge
bbctl config --type bridgev2 sh-simplex

# 4. Copy the as_token and hs_token from the generated config into your
#    NixOS services.mautrix-simplex.settings.appservice config

# 5. Rebuild NixOS
sudo nixos-rebuild switch
```

## Limitations

- **Single writer**: Each simplex-chat database can only be used by one bridge instance at a time
- **Reactions**: SimpleX only supports 8 specific emoji reactions (`👍👎😀😂😢❤🚀✅`); other emoji are silently dropped
- **No typing indicators**: SimpleX doesn't expose typing status via the chat API
- **No presence**: Presence/online status is not bridged
- **No read receipts**: Read receipt bridging is not yet implemented

## License

AGPL-3.0-or-later
