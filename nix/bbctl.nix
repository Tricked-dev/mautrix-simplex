{
  lib,
  buildGoModule,
  fetchFromGitHub,
}:

buildGoModule rec {
  pname = "bbctl";
  version = "0.13.0";

  src = fetchFromGitHub {
    owner = "beeper";
    repo = "bridge-manager";
    rev = "v${version}";
    hash = "sha256-bNnansZNshWp70LQQsa6+bS+LJxpCzdTkL2pX+ksrP0=";
  };

  vendorHash = "sha256-yTNUxwnulQ+WbHdQbeNDghH4RPXurQMIgKDyXfrMxG8=";

  ldflags = [
    "-s"
    "-w"
    "-X main.Tag=v${version}"
  ];

  doCheck = false;

  meta = {
    description = "Beeper bridge manager (bbctl) for self-hosting bridges";
    homepage = "https://github.com/beeper/bridge-manager";
    license = lib.licenses.asl20;
    mainProgram = "bbctl";
  };
}
