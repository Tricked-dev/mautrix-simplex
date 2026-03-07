{
  description = "mautrix-simplex - A Matrix-SimpleX puppeting bridge";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    nix2container.url = "github:nlewo/nix2container";
    nix2container.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      nix2container,
    }:
    {
      nixosModules.default = import ./nix/module.nix;
    }
    // flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.permittedInsecurePackages = [ "olm-3.2.16" ];
        };
        n2c = nix2container.packages.${system}.nix2container;
        simplex-chat = pkgs.callPackage ./nix/simplex-chat.nix { };
        mautrix-simplex = pkgs.callPackage ./nix/package.nix { };
        bbctl = pkgs.callPackage ./nix/bbctl.nix { };
        mautrix-webhook = pkgs.callPackage ./nix/webhook-package.nix { };
        dockerImage = n2c.buildImage {
          name = "mautrix-simplex";
          tag = "latest";
          copyToRoot = pkgs.buildEnv {
            name = "root";
            paths = [
              mautrix-simplex
              pkgs.cacert
              pkgs.ffmpeg
            ];
            pathsToLink = [ "/bin" "/etc" ];
          };
          config = {
            Cmd = [ "/bin/mautrix-simplex" "-c" "/data/config.yaml" ];
            WorkingDir = "/data";
            Env = [ "HOME=/data" ];
            ExposedPorts = { "29340/tcp" = { }; };
            Volumes = { "/data" = { }; };
          };
        };
        dockerImageBundled = n2c.buildImage {
          name = "mautrix-simplex";
          tag = "with-simplex";
          copyToRoot = pkgs.buildEnv {
            name = "root";
            paths = [
              mautrix-simplex
              simplex-chat
              pkgs.cacert
              pkgs.ffmpeg
            ];
            pathsToLink = [ "/bin" "/etc" ];
          };
          config = {
            Cmd = [ "/bin/mautrix-simplex" "-c" "/data/config.yaml" ];
            WorkingDir = "/data";
            Env = [ "HOME=/data" ];
            ExposedPorts = { "29340/tcp" = { }; };
            Volumes = { "/data" = { }; };
          };
        };
        dockerImageSimplex = n2c.buildImage {
          name = "simplex-chat";
          tag = "latest";
          copyToRoot = pkgs.buildEnv {
            name = "root";
            paths = [
              simplex-chat
              pkgs.cacert
            ];
            pathsToLink = [ "/bin" "/etc" ];
          };
          config = {
            Cmd = [ "/bin/simplex-chat" ];
            WorkingDir = "/data";
            Env = [ "HOME=/data" ];
            Volumes = { "/data" = { }; };
          };
        };
        dockerImageWebhook = n2c.buildImage {
          name = "mautrix-webhook";
          tag = "latest";
          copyToRoot = pkgs.buildEnv {
            name = "root";
            paths = [
              mautrix-webhook
              pkgs.cacert
            ];
            pathsToLink = [ "/bin" "/etc" ];
          };
          config = {
            Cmd = [ "/bin/mautrix-webhook" "-c" "/data/config.yaml" "--no-update" ];
            WorkingDir = "/data";
            Env = [ "HOME=/data" ];
            Volumes = { "/data" = { }; };
          };
        };
      in
      {
        packages = {
          inherit mautrix-simplex mautrix-webhook simplex-chat bbctl dockerImage dockerImageBundled dockerImageSimplex dockerImageWebhook;
          default = mautrix-simplex;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gcc
            pkg-config
            olm
            sqlite
          ];

          shellHook = ''
            export CGO_ENABLED=1
            echo "mautrix-simplex dev shell"
          '';
        };
      }
    );
}
