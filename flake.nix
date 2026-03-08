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
        pkgs = import nixpkgs { inherit system; };
        # Static packages: use pkgsStatic on Linux so Docker image binaries are
        # linked against musl (no glibc closure).  Falls back to regular pkgs on
        # macOS where fully-static binaries are not supported.
        staticPkgs = if pkgs.stdenv.isLinux then pkgs.pkgsStatic else pkgs;
        n2c = nix2container.packages.${system}.nix2container;
        simplex-chat = pkgs.callPackage ./nix/simplex-chat.nix { };
        mautrix-simplex = pkgs.callPackage ./nix/package.nix { };
        bbctl = pkgs.callPackage ./nix/bbctl.nix { };
        mautrix-webhook = pkgs.callPackage ./nix/webhook-package.nix { };
        # Static variants used inside Docker images — same derivations built in
        # the pkgsStatic environment so the resulting binaries carry no glibc dep.
        mautrix-simplex-static = staticPkgs.callPackage ./nix/package.nix { };
        mautrix-webhook-static = staticPkgs.callPackage ./nix/webhook-package.nix { };
        # Minimal ffmpeg: keeps only ffmpeg's internal decoders (h264, hevc, vp8/9,
        # av1, etc. are built into libavcodec without external libraries) and the
        # built-in mjpeg encoder for thumbnail output. All heavy external codec
        # libraries (libx264, libx265, libvpx, libopus, libvorbis, libass…) are
        # removed. Built via staticPkgs so the binary carries no glibc closure.
        minimalFfmpeg = (staticPkgs.ffmpeg-headless.override {
          withAmf = false;
          withAom = false;
          withAss = false;
          withBluray = false;
          withCudaLLVM = false;
          withCuvid = false;
          withDrm = false;
          withFontconfig = false;
          withFreetype = false;
          withFribidi = false;
          withGnutls = false;
          withMp3lame = false;
          withNvcodec = false;
          withOpencl = false;
          withOpenjpeg = false;
          withOpenmpt = false;
          withOpus = false;
          withSoxr = false;
          withSrt = false;
          withSsh = false;
          withSvtav1 = false;
          withTheora = false;
          withVidStab = false;
          withVorbis = false;
          withVpx = false;
          withVulkan = false;
          withWebp = false;
          withX264 = false;
          withX265 = false;
          withXvid = false;
          withZimg = false;
          buildFfplay = false;
          buildAvdevice = false;
          buildPostproc = false;
        }).overrideAttrs (old: {
          configureFlags = (old.configureFlags or [ ]) ++ [
            "--disable-encoders"
            "--enable-encoder=mjpeg"
            "--disable-muxers"
            "--enable-muxer=image2"
            "--disable-protocols"
            "--enable-protocol=file"
            "--disable-bsfs"
          ];
        });
        dockerImage = n2c.buildImage {
          name = "mautrix-simplex";
          tag = "latest";
          copyToRoot = pkgs.buildEnv {
            name = "root";
            paths = [
              mautrix-simplex-static
              pkgs.cacert
              minimalFfmpeg
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
              mautrix-simplex-static
              simplex-chat
              pkgs.cacert
              minimalFfmpeg
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
              mautrix-webhook-static
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
          inherit mautrix-simplex mautrix-webhook simplex-chat bbctl minimalFfmpeg dockerImage dockerImageBundled dockerImageSimplex dockerImageWebhook;
          default = mautrix-simplex;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gcc
            pkg-config
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
