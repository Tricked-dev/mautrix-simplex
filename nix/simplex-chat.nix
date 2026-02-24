{
  lib,
  stdenv,
  fetchurl,
  autoPatchelfHook,
  zlib,
  openssl,
  gmp,
}:

let
  version = "6.5.0-beta.5";
  srcs = {
    x86_64-linux = {
      url = "https://github.com/simplex-chat/simplex-chat/releases/download/v${version}/simplex-chat-ubuntu-24_04-x86_64";
      hash = "sha256-dOhV1KpfDvVTxt4Cfvd8Fdc6tKQJ8wa9zK3aPeNHw14=";
    };
    aarch64-linux = {
      url = "https://github.com/simplex-chat/simplex-chat/releases/download/v${version}/simplex-chat-ubuntu-24_04-aarch64";
      hash = "sha256-wplVhgISqxSWftSjfU9EIRgRpKUE9dTlkv1c8Lr3ctQ=";
    };
  };
in
stdenv.mkDerivation {
  pname = "simplex-chat";
  inherit version;

  src = fetchurl srcs.${stdenv.hostPlatform.system};

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
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
    ];
  };
}
