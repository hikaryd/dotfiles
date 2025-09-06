{ pkgs, ... }: {
  programs.vscode = {
    enable = false;
    package = pkgs.vscode;

    profiles = {
      review = {
        userSettings = {
          # Старт/телеметрия
          "workbench.startupEditor" = "none";
          "telemetry.telemetryLevel" = "off";

          # Минималистичный UI
          "workbench.activityBar.location" = "hidden";
          "workbench.statusBar.visible" = false;
          "workbench.editor.showTabs" = "single";
          "workbench.editor.showIcons" = false;
          "breadcrumbs.enabled" = false;
          "window.commandCenter" = false;
          "window.titleBarStyle" = "native";

          # Тема (тёмная Ayu)
          "workbench.colorTheme" = "Ayu Mirage"; # или "Ayu Dark"

          # Редактор: ничего лишнего
          "editor.minimap.enabled" = false;
          "editor.hover.enabled" = false;
          "editor.wordWrap" = "off";
          "editor.renderLineHighlight" = "none";
          "editor.stickyScroll.enabled" = false;
          "editor.bracketPairColorization.enabled" = false;

          # Дифф для ревью
          "diffEditor.renderSideBySide" = true;
          "diffEditor.ignoreTrimWhitespace" = true;
          "diffEditor.experimental.collapseUnchangedRegions" = true;
          "diffEditor.showMove" = true;

          # Git
          "git.autofetch" = true;
          "git.autorefresh" = true;
          "git.openRepositoryInParentFolders" = "always";

          # Vim-mode (VSCodeVim)
          "vim.useSystemClipboard" = true;
          "vim.useCtrlKeys" = true;
          "vim.hlsearch" = false;
          "vim.incsearch" = true;
          # Быстрая навигация по изменениям как в vim: ]c / [c
          "vim.normalModeKeyBindingsNonRecursive" = [
            {
              "before" = [ "]" "c" ];
              "commands" = [ "editor.action.diffReview.next" ];
            }
            {
              "before" = [ "[" "c" ];
              "commands" = [ "editor.action.diffReview.prev" ];
            }
          ];

          # Если у тебя self-hosted GitLab:
          # "gitlab.instanceUrl" = "https://gitlab.<твой-домен>";
        };

        keybindings = [
          # Навигация по изменениям/комментам в diff’е
          {
            key = "f7";
            command = "editor.action.diffReview.next";
          }
          {
            key = "shift+f7";
            command = "editor.action.diffReview.prev";
          }

          # Открыть список MR из GitLab Workflow
          {
            key = "cmd+shift+m";
            command = "gitlab.showIssuesAndMergeRequests";
          }

          # Прятать/показывать боковую панель (activity bar скрыта, живём хоткеями)
          {
            key = "cmd+\\";
            command = "workbench.action.toggleSidebarVisibility";
          }

          # Палитра команд на всякий
          {
            key = "cmd+shift+p";
            command = "workbench.action.showCommands";
          }
        ];

        extensions = with pkgs.vscode-extensions; [
          gitlab.gitlab-workflow
          vscodevim.vim
          teabyii.ayu
          # (иконки не нужны, но хочешь — добавь pkief.material-icon-theme)
        ];
      };
    };
  };
}
