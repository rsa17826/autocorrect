{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };

        pythonEnv = pkgs.python313.withPackages (
          ps: with ps; [
            evdev
          ]
        );
        myApp = pkgs.writeShellScriptBin "autocorrect" ''
          export PATH="${pkgs.lib.makeBinPath [ pkgs.wtype ]}:$PATH"
          ${pythonEnv}/bin/python ${./autocorrect.py} ${./corrections.json} "$@"
        '';
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            pythonEnv
            wtype
          ];
        };
        packages.default = myApp;
      }
    );
}
