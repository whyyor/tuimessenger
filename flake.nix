{
  description = "tuimessenger: WhatsApp, Telegram, Slack and Instagram in one terminal app (local bridges, no Matrix server)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      version = "0.1.0-dev+${self.shortRev or self.dirtyShortRev or "unknown"}";
    in
    {
      packages = forAllSystems (pkgs: rec {
        tuimessenger = pkgs.buildGoModule {
          pname = "tuimessenger";
          inherit version;
          src = self; # tracked files only
          vendorHash = "sha256-9CzeicphsfyAvZx2OnoiqtNfIVIbtQ47E67FweSZet4="; # scripts/update-vendor-hash.sh after go.mod/go.sum change
          subPackages = [ "cmd/tuimessenger" ];
          # goolm: pure-Go olm; sqlite_fts5: full-text search in the client database. cgo builds sqlite and webp.
          tags = [ "goolm" "sqlite_fts5" ];
          env.CGO_ENABLED = 1;
          ldflags = [ "-s" "-w" "-X main.version=${version}" ];
          doCheck = false; # CI runs the tests
          meta = {
            description = "WhatsApp, Telegram, Slack and Instagram in one terminal app";
            homepage = "https://github.com/whyyor/tuimessenger";
            license = pkgs.lib.licenses.agpl3Plus;
            mainProgram = "tuimessenger";
            platforms = systems;
          };
        };
        default = tuimessenger;
      });
    };
}
