{
  buildGoModule,
}:

buildGoModule {
  pname = "mautrix-webhook";
  version = "0.1.0";

  src = ./..;

  vendorHash = "sha256-Fn97FYYAKqzK941150b6d9OCW7Gi4sFQ1T0KIVuGMMQ=";

  env.CGO_ENABLED = "1";

  # goolm: pure-Go OLM implementation — no libolm CGO dependency.
  # go-sqlite3 still requires CGO but compiles sqlite3.c in-tree with no
  # external shared libraries, so the binary can be linked fully statically.
  tags = [ "goolm" ];

  subPackages = [ "cmd/mautrix-webhook" ];

  doCheck = false;

  meta.mainProgram = "mautrix-webhook";
}
