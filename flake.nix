{
  description = "mautrix-simplex - A Matrix-SimpleX puppeting bridge";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
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
        simplex-chat = pkgs.callPackage ./nix/simplex-chat.nix { };
        mautrix-simplex = pkgs.callPackage ./nix/package.nix { };
        bbctl = pkgs.callPackage ./nix/bbctl.nix { };
        dockerImage = pkgs.dockerTools.buildLayeredImage {
          name = "mautrix-simplex";
          tag = "latest";
          contents = [
            mautrix-simplex
            pkgs.cacert
            pkgs.ffmpeg
          ];
          config = {
            Cmd = [ "/bin/mautrix-simplex" "-c" "/data/config.yaml" ];
            WorkingDir = "/data";
            Env = [ "HOME=/data" ];
            ExposedPorts = { "29340/tcp" = { }; };
            Volumes = { "/data" = { }; };
          };
        };
      in
      {
        packages = {
          inherit mautrix-simplex simplex-chat bbctl dockerImage;
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
