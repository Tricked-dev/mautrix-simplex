{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.mautrix-simplex;
  settingsFormat = pkgs.formats.yaml { };
  settingsFile = settingsFormat.generate "config.yaml" cfg.settings;
  dataDir = cfg.dataDir;
in
{
  options.services.mautrix-simplex = {
    enable = lib.mkEnableOption "mautrix-simplex, a Matrix-SimpleX puppeting bridge";

    package = lib.mkPackageOption pkgs "mautrix-simplex" { };

    simplexChatPackage = lib.mkOption {
      type = lib.types.package;
      description = "The simplex-chat package to use for the companion service.";
    };

    dataDir = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/mautrix-simplex";
      example = "/var/lib/selfhosted/matrix/mautrix-simplex";
      description = "Base directory for all mautrix-simplex state (database, simplex-chat data, files).";
    };

    settings = lib.mkOption {
      type = settingsFormat.type;
      default = { };
      description = ''
        Bridge configuration. Converted to YAML and written to config.yaml.
        See the example config for available options.
      '';
    };

    serviceDependencies = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = lib.optional config.services.matrix-synapse.enable config.services.matrix-synapse.serviceUnit;
      defaultText = lib.literalExpression ''
        lib.optional config.services.matrix-synapse.enable config.services.matrix-synapse.serviceUnit
      '';
      description = "Systemd units to require and wait for before starting the bridge.";
    };

    registerToSynapse = lib.mkOption {
      type = lib.types.bool;
      default = config.services.matrix-synapse.enable;
      defaultText = lib.literalExpression "config.services.matrix-synapse.enable";
      description = "Whether to automatically register the appservice with Synapse.";
    };

    simplexChatPort = lib.mkOption {
      type = lib.types.port;
      default = 5225;
      description = "Port for the companion simplex-chat WebSocket API.";
    };

    filesFolder = lib.mkOption {
      type = lib.types.str;
      default = "${cfg.dataDir}/files";
      defaultText = lib.literalExpression ''"''${cfg.dataDir}/files"'';
      description = "Directory for simplex-chat file storage. Must match the network.files_folder config.";
    };
  };

  config = lib.mkIf cfg.enable {
    services.mautrix-simplex.settings = {
      homeserver = lib.mkDefault {
        software = "standard";
      };
      appservice = lib.mkDefault {
        database = {
          type = "sqlite3-fk-wal";
          uri = "${dataDir}/mautrix-simplex.db";
        };
        id = "simplex";
        bot = {
          username = "simplexbot";
          displayname = "SimpleX Bridge Bot";
        };
        as_token = "";
        hs_token = "";
      };
      bridge = lib.mkDefault {
        command_prefix = "!simplex";
      };
      network = {
        displayname_template = lib.mkDefault "{{.DisplayName}} (SimpleX)";
        simplex_binary = lib.mkDefault "${cfg.simplexChatPackage}/bin/simplex-chat";
        files_folder = lib.mkDefault cfg.filesFolder;
      };
    };

    # Create the dataDir if it's not under /var/lib (StateDirectory only handles /var/lib).
    systemd.tmpfiles.rules = lib.mkIf (!lib.hasPrefix "/var/lib/" "${dataDir}/") [
      "d ${dataDir} 0750 mautrix-simplex mautrix-simplex -"
      "d ${cfg.filesFolder} 0750 mautrix-simplex mautrix-simplex -"
      "d ${cfg.filesFolder}/tmp 0750 mautrix-simplex mautrix-simplex -"
    ];

    users = lib.mkIf (!lib.hasPrefix "/var/lib/" "${dataDir}/") {
      users.mautrix-simplex = {
        isSystemUser = true;
        group = "mautrix-simplex";
        home = dataDir;
      };
      groups.mautrix-simplex = { };
    };

    systemd.services.simplex-chat = {
      description = "SimpleX Chat companion for mautrix-simplex";
      wantedBy = [ "multi-user.target" ];
      before = [ "mautrix-simplex.service" ];
      serviceConfig =
        {
          Type = "simple";
          WorkingDirectory = dataDir;
          ExecStart = lib.concatStringsSep " " [
            "${cfg.simplexChatPackage}/bin/simplex-chat"
            "-p ${toString cfg.simplexChatPort}"
            "-d ${dataDir}/simplex-data"
            "--files-folder ${cfg.filesFolder}"
            "--temp-folder ${cfg.filesFolder}/tmp"
          ];
          Restart = "on-failure";
          RestartSec = 5;
          ReadWritePaths = [ dataDir ];

          # Sandboxing
          NoNewPrivileges = true;
          ProtectSystem = "strict";
          ProtectHome = true;
          PrivateTmp = true;
          PrivateDevices = true;
          ProtectKernelTunables = true;
          ProtectControlGroups = true;
          RestrictSUIDSGID = true;

          # Restrict WebSocket API to localhost only.
          # simplex-chat has no built-in authentication for its WebSocket API,
          # so we use systemd's IP filtering to ensure only local connections.
          IPAddressAllow = "localhost";
          IPAddressDeny = "any";
        }
        // (
          if lib.hasPrefix "/var/lib/" "${dataDir}/" then
            {
              DynamicUser = true;
              StateDirectory = lib.removePrefix "/var/lib/" dataDir;
            }
          else
            {
              User = "mautrix-simplex";
              Group = "mautrix-simplex";
            }
        );
    };

    systemd.services.mautrix-simplex = {
      description = "Matrix-SimpleX puppeting bridge";
      wantedBy = [ "multi-user.target" ];
      wants = [ "network-online.target" "simplex-chat.service" ];
      after = [ "network-online.target" "simplex-chat.service" ] ++ cfg.serviceDependencies;
      requires = [ "simplex-chat.service" ];

      serviceConfig =
        {
          Type = "simple";
          WorkingDirectory = dataDir;
          ExecStart = "${cfg.package}/bin/mautrix-simplex -c ${settingsFile} --no-update";
          Restart = "on-failure";
          RestartSec = 5;
          ReadWritePaths = [ dataDir ];

          # Sandboxing
          NoNewPrivileges = true;
          ProtectSystem = "strict";
          ProtectHome = true;
          PrivateTmp = true;
          PrivateDevices = true;
          ProtectKernelTunables = true;
          ProtectControlGroups = true;
          RestrictSUIDSGID = true;
        }
        // (
          if lib.hasPrefix "/var/lib/" "${dataDir}/" then
            {
              DynamicUser = true;
              StateDirectory = lib.removePrefix "/var/lib/" dataDir;
            }
          else
            {
              User = "mautrix-simplex";
              Group = "mautrix-simplex";
            }
        );
    };

    services.matrix-synapse = lib.mkIf cfg.registerToSynapse {
      settings.app_service_config_files = [
        "${dataDir}/registration.yaml"
      ];
    };
  };
}
