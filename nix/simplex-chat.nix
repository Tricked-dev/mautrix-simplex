{
  lib,
  stdenv,
  fetchurl,
  autoPatchelfHook,
  zlib,
  openssl,
  gmp,
}:

stdenv.mkDerivation {
  pname = "simplex-chat";
  version = "6.5.0-beta.5";

  src = fetchurl {
    url = "https://github.com/simplex-chat/simplex-chat/releases/download/v6.5.0-beta.5/simplex-chat-ubuntu-22_04-x86_64";
    hash = "sha256-kUy7NXOsCpi/vDwXszpetX/PKQ/e1zD/2v+rnhu1voU=";
  };

  nativeBuildInputs = [ autoPatchelfHook ];
  buildInputs = [
    stdenv.cc.cc.lib
    zlib
    openssl
    gmp
  ];

  unpackPhase = "true";
  installPhase = ''
    mkdir -p $out/bin
    cp $src $out/bin/simplex-chat
    chmod +x $out/bin/simplex-chat
  '';

  meta = {
    description = "SimpleX Chat CLI";
    homepage = "https://github.com/simplex-chat/simplex-chat";
    license = lib.licenses.agpl3Plus;
    mainProgram = "simplex-chat";
    platforms = [ "x86_64-linux" ];
  };
}
