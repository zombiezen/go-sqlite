{ pkgs ? import (fetchTarball "https://github.com/NixOS/nixpkgs/archive/0e857e0089d78dee29818dc92722a72f1dea506f.tar.gz") {}
}:

pkgs.mkShell {
  packages = [
    pkgs.go_1_19
  ];
}
