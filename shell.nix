with import (
  fetchTarball {
    url = https://github.com/nixos/nixpkgs-channels/archive/206a1c00baea8678dcce8c75dcc3df48ba59539b.tar.gz;
    sha256 = "1agalbjq05fzfyf6bf8rvaj0r8hnvwbbvmkdlq7smvb89krf9dkh";
  }
) {};
mkShell {
  buildInputs = [ go_1_11 pkgconfig zeromq ];
  shellHook = ''
    unset GOPATH
    export GO111MODULE=on
  '';
}
