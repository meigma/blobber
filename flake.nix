{
  description = "Blobber - Push and pull files to OCI container registries";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        # Version info from git
        version = if (self ? shortRev) then self.shortRev else "dev";
        commit = if (self ? rev) then self.rev else "none";
        date = if (self ? lastModifiedDate) then self.lastModifiedDate else "unknown";
      in
      {
        packages = {
          blobber = pkgs.buildGoModule {
            pname = "blobber";
            inherit version;

            src = ./.;

            vendorHash = "sha256-9/FbGkeufznKIYx19DZZfJqcAmH47jb176n/bvOA5YE=";

            subPackages = [ "cmd/blobber" ];

            ldflags = [
              "-s"
              "-w"
              "-X github.com/meigma/blobber/cmd/blobber/cli.version=${version}"
              "-X github.com/meigma/blobber/cmd/blobber/cli.commit=${commit}"
              "-X github.com/meigma/blobber/cmd/blobber/cli.date=${date}"
            ];

            meta = with pkgs.lib; {
              description = "Push and pull files to OCI container registries";
              homepage = "https://github.com/meigma/blobber";
              license = licenses.mit;
              maintainers = [ ];
              mainProgram = "blobber";
            };
          };

          default = self.packages.${system}.blobber;
        };

        apps = {
          blobber = flake-utils.lib.mkApp {
            drv = self.packages.${system}.blobber;
          };
          default = self.apps.${system}.blobber;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_24
            golangci-lint
            just
          ];
        };
      }
    );
}
