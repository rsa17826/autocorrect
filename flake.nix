{
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
            vendorHash = "sha256-UPtcoi4AXtmyURoMme0GNKo/hiMX9yxcwQqYIKgnYPY=";
          };
        };
        devShells = {
          # Development environment
          default = pkgs.mkShell {
            hardeningDisable = [ "fortify" ];
            buildInputs = with pkgs; [
              go
              gopls
            ];
          };
        };
      }
    );
}
