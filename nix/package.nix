{
  lib,
  buildGoModule,
  olm,
}:

buildGoModule {
  pname = "mautrix-simplex";
  version = "0.1.0";

  src = ./..;

  vendorHash = "sha256-Fn97FYYAKqzK941150b6d9OCW7Gi4sFQ1T0KIVuGMMQ=";

  env.CGO_ENABLED = "1";

  buildInputs = [ olm ];

  subPackages = [ "cmd/mautrix-simplex" ];

  ldflags = [
    "-s"
    "-w"
    "-X main.Tag=v0.1.0"
  ];

  doCheck = false;

  meta = {
    description = "A Matrix-SimpleX puppeting bridge";
    homepage = "https://github.com/mautrix/simplex";
    license = lib.licenses.agpl3Plus;
    mainProgram = "mautrix-simplex";
  };
}
