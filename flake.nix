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
        mautrix-simplex = pkgs.callPackage ./nix/package.nix { };
        bbctl = pkgs.callPackage ./nix/bbctl.nix { };
      in
      {
        packages = {
          inherit mautrix-simplex bbctl;
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
