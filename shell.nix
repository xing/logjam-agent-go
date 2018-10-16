with builtins;

let
  pkgs = import (fetchGit {
    url = https://github.com/NixOS/nixpkgs;
    rev = "6a1b65b60691ea025acab758f06891fbbe4cc148";
  }) {
    config = {};
    overlays = [
      (self: super: {
        go = super.go_1_10;
      })
    ];
  };

  inherit (pkgs.lib) hasPrefix splitString concatMapStrings;

  goDeps = pkgs.stdenv.mkDerivation {
    name = "goDeps";
    src = ./Gopkg.lock;
    phases = "buildPhase";
    buildInputs = [ pkgs.remarshal ];
    buildPhase = ''
      remarshal --indent-json -if toml -i $src -of json -o $out
    '';
  };

  fixUrl = name:
    if (hasPrefix "golang.org" name) then
      "https://go.googlesource.com/" + (elemAt (splitString "/" name) 2)
    else
      if (hasPrefix "google.golang.org" name) then
        "https://github.com/golang/" + (elemAt (splitString "/" name) 1)
      else
        "https://" + name;

  projects = (fromJSON (readFile goDeps.out)).projects;

  mkProject = project:
    pkgs.stdenv.mkDerivation {
      name = replaceStrings ["/"] ["-"] project.name;

      src = fetchGit {
        url = fixUrl project.name;
        rev = project.revision;
      } // (if project?branch then { ref = project.branch; } else {});

      phases = [ "buildPhase" ];

      buildPhase = ''
        mkdir -p $out/package
        cp -r $src/* $out/package
        echo "${project.name}" > $out/name
      '';
    };

  projectSources = map mkProject projects;

  depTree = pkgs.stdenv.mkDerivation {
    name = "depTree";

    src = projectSources;

    phases = [ "buildPhase" ];

    buildPhase = ''
      mkdir -p $out
      for pkg in $src; do
        echo building "$pkg"
        name="$(cat $pkg/name)"
        mkdir -p "$out/vendor/$name"
        cp -r $pkg/package/* "$out/vendor/$name"
      done
    '';
  };
in

with pkgs;

mkShell {
  buildInputs = [
    go
    dep
    godef
    zeromq4
    pkgconfig
    gotools
    goimports
    clang
  ];

  CGO_ENABLED = "1";
  GOPATH = "/home/manveru/go";
  GOROOT = "${go}/share/go";

  shellHook = ''
    export GOPATH="$HOME/go";
    export GOROOT="${go}/share/go"
    if [[ -e shell.nix ]]; then
      set -x
      rm -rf vendor
      ln -s ${depTree}/vendor $PWD/vendor
      set +x
    fi
  '';
}
