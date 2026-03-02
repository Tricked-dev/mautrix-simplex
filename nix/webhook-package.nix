{
  buildGoModule,
  olm,
}:

buildGoModule {
  pname = "mautrix-webhook";
  version = "0.1.0";

  src = ./..;

  vendorHash = "sha256-Fn97FYYAKqzK941150b6d9OCW7Gi4sFQ1T0KIVuGMMQ=";

  env.CGO_ENABLED = "1";

  buildInputs = [ olm ];

  subPackages = [ "cmd/mautrix-webhook" ];

  doCheck = false;

  meta.mainProgram = "mautrix-webhook";
}
