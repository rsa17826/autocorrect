{
  description = "An input manager to prevent having to chain devices and allow wasily unlocking keyboard";

  inputs = {
    nixpkgs = {
      url = "github:NixOS/nixpkgs/nixos-unstable";
    };
    flake-utils = {
      url = "github:numtide/flake-utils";
    };
  };

  outputs =
    {
      nixpkgs,
      flake-utils,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        packages = {
          # The actual package
          default = pkgs.buildGoModule {
            pname = "autocorrect";
            version = "3";
            src = ./.;
            vendorHash = "sha256-2Nu6sYNREjq1WxrGF1BvI8JbMjhipfh/w5D/040jUdo=";
          };
        };
        devShells = {
          # Development environment
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
              go
              gopls
            ];
          };
        };
      }
    );
}
