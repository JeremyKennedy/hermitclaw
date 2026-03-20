{
  description = "hermitclaw: persistent Claude Code agent runner";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
    in
    {
      packages.${system}.default = pkgs.buildGoModule {
        pname = "hermitclaw";
        version = "0.1.0";
        src = ./.;
        vendorHash = null; # will be updated after first build
        doCheck = true;
        nativeBuildInputs = [
          pkgs.makeWrapper
          pkgs.installShellFiles
        ];
        postInstall = ''
          wrapProgram $out/bin/hermitclaw \
            --suffix PATH : ${pkgs.lib.makeBinPath [ pkgs.tmux ]}
          installShellCompletion --cmd hermitclaw \
            --fish <($out/bin/hermitclaw completion fish) \
            --bash <($out/bin/hermitclaw completion bash) \
            --zsh <($out/bin/hermitclaw completion zsh)
        '';
        meta = {
          description = "Persistent Claude Code agent runner";
          mainProgram = "hermitclaw";
        };
      };

      homeManagerModules.default =
        {
          config,
          lib,
          pkgs,
          ...
        }:
        let
          cfg = config.programs.hermitclaw;
          hermitclaw = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
          channelsStr = lib.concatStringsSep "," cfg.channels;
          extraArgsStr = lib.concatStringsSep " " cfg.extraArgs;
        in
        {
          options.programs.hermitclaw = {
            enable = lib.mkEnableOption "hermitclaw persistent Claude Code runner";

            claudePackage = lib.mkOption {
              type = lib.types.package;
              description = "The claude-code package to use.";
            };

            workingDirectory = lib.mkOption {
              type = lib.types.str;
              default = "~/dev";
              description = "Working directory for Claude Code sessions.";
            };

            channels = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              default = [ ];
              description = "Claude Code channels to connect.";
            };

            extraArgs = lib.mkOption {
              type = lib.types.listOf lib.types.str;
              default = [ ];
              description = "Extra arguments passed to claude.";
            };

            sessionName = lib.mkOption {
              type = lib.types.str;
              default = "hermitclaw";
              description = "tmux session name and systemd service name.";
            };

            environment = lib.mkOption {
              type = lib.types.attrsOf lib.types.str;
              default = { };
              description = "Environment variables for the service.";
            };

            autoResume = lib.mkOption {
              type = lib.types.bool;
              default = true;
              description = "Resume previous conversation on restart.";
            };

            restartDelay = lib.mkOption {
              type = lib.types.int;
              default = 5;
              description = "Seconds to wait before restarting after Claude exits.";
            };
          };

          config = lib.mkIf cfg.enable {
            assertions = [
              {
                assertion = config.programs ? claude-code;
                message = "hermitclaw requires programs.claude-code (from home-manager).";
              }
            ];

            home.packages = [ hermitclaw ];

            xdg.configFile."hermitclaw/config.toml".text = ''
              working_directory = "${cfg.workingDirectory}"
              session_name = "${cfg.sessionName}"
              claude_binary = "${cfg.claudePackage}/bin/claude"
              channels = "${channelsStr}"
              extra_args = "${extraArgsStr}"
              auto_resume = ${lib.boolToString cfg.autoResume}
              restart_delay = ${toString cfg.restartDelay}
            '';

            systemd.user.services.${cfg.sessionName} = {
              Unit = {
                Description = "Hermitclaw persistent Claude Code runner";
                After = [ "default.target" ];
              };
              Service = {
                Type = "oneshot";
                RemainAfterExit = true;
                Environment =
                  [ "TMUX_TMPDIR=%t" ]
                  ++ (lib.mapAttrsToList (k: v: "${k}=${v}") cfg.environment);
                ExecStart = "${hermitclaw}/bin/hermitclaw start";
                ExecStop = "${hermitclaw}/bin/hermitclaw stop";
              };
            };

            programs.claude-code = {
              skills.welcome = lib.mkDefault ./skills/welcome;

              settings.hooks.SessionStart = lib.mkDefault [
                {
                  matcher = "startup";
                  hooks = [
                    {
                      type = "command";
                      command = ''echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"Run /welcome now to announce this session on Telegram."}}'  '';
                      async = false;
                    }
                  ];
                }
              ];
            };
          };
        };

      checks.${system} = {
        build = self.packages.${system}.default;
      };
    };
}
