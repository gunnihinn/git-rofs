let
  sources = import ./nix/sources.nix;
  pkgs = import sources.nixpkgs {};
in
pkgs.mkShell {
  buildInputs = [
    pkgs.go
    pkgs.libgit2
    pkgs.pkg-config
  ];

  GOPATH="$(pwd)/.go";
}
