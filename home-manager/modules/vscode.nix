{ pkgs, lib, ... }: {
  programs.vscode = {
    enable = true;
    package = pkgs.vscode;
    profiles.default = {
      userSettings = {
        # Debug:
        "debug.allowBreakpointsEverywhere" = true;
        "debug.javascript.codelens.npmScripts" = "never";
        "editor.glyphMargin" = false; # hide breakpoints
        "debug.console.closeOnEnd" = true;

        # Editor:
        "editor.cursorBlinking" = "solid";
        "editor.accessibilitySupport" = "off";
        "editor.codeLens" = true;
        "editor.colorDecorators" = false;
        "editor.dragAndDrop" = false;
        "editor.folding" =
          false; # change to `true` and use `cmd+alt` + `[` or `]`
        "editor.showFoldingControls" = "mouseover";
        "editor.fontFamily" = lib.mkForce "Fira Code";
        "editor.fontLigatures" = true;
        "editor.fontSize" = lib.mkForce 17;
        "explorer.openEditors.visible" = 1;
        "editor.minimap.enabled" = false;
        "editor.renderWhitespace" = "selection";
        "editor.renderLineHighlight" = "none";
        "editor.hideCursorInOverviewRuler" = false;
        "explorer.compactFolders" = false;
        "editor.occurrencesHighlight" = "off";
        "editor.overviewRulerBorder" = false;
        "editor.bracketPairColorization.enabled" = false;
        "editor.rulers" = [ 80 ];
        "editor.scrollBeyondLastLine" = false;
        "editor.snippetSuggestions" = "none";
        "editor.cursorSurroundingLines" = 3;
        "editor.useTabStops" = true;
        "editor.formatOnPaste" = false;
        "editor.wordWrap" = "on";
        "editor.wordWrapColumn" = 80;
        "editor.hover.enabled" = false;
        "editor.stickyScroll.enabled" = false;
        "explorer.confirmDelete" = false;
        "explorer.confirmDragAndDrop" = false;
        "explorer.decorations.badges" = false;
        "extensions.ignoreRecommendations" = true;
        "diffEditor.renderSideBySide" = false;
        "breadcrumbs.enabled" = false;
        "window.commandCenter" = false;
        "workbench.activityBar.location" = "hidden";
        "workbench.editor.tabActionCloseVisibility" = false;
        "workbench.hover.delay" = 10000; # ms, TODO: find way to turn this off
        "editor.scrollbar.vertical" = "auto";
        "editor.scrollbar.horizontal" = "hidden";

        # Use simple navigation:
        "editor.gotoLocation.multipleReferences" = "peek";
        "editor.gotoLocation.multipleDefinitions" = "peek";
        "editor.gotoLocation.multipleDeclarations" = "peek";
        "editor.gotoLocation.multipleImplementations" = "peek";
        "editor.gotoLocation.multipleTypeDefinitions" = "peek";

        # Do not modify my code; please:
        "editor.autoClosingBrackets" = "never";
        "editor.autoClosingQuotes" = "never";
        "editor.autoSurround" = "never";
        "editor.autoClosingOvertype" = "never";
        "editor.suggestOnTriggerCharacters" = false;
        "editor.acceptSuggestionOnCommitCharacter" = false;
        "editor.acceptSuggestionOnEnter" = "on";
        "editor.parameterHints.enabled" = false;
        "editor.tabCompletion" = "on";
        "editor.suggestSelection" = "recentlyUsed";
        "editor.quickSuggestions" = {
          "other" = false; # change to "true" to enable regular code suggetions
          "comments" = false;
          "strings" = false;
        };
        "editor.formatOnType" = false;
        "editor.inlineSuggest.enabled" = false;
        "editor.suggest.localityBonus" = true;
        "editor.lightbulb.enabled" = "off";
        "editor.detectIndentation" = false;
        "editor.guides.indentation" = false;
        "editor.showUnused" = false;
        "editor.pasteAs.preferences" = [ "text.plain" ];

        # Search
        "search.quickAccess.preserveInput" = true;
        "search.quickOpen.includeSymbols" = false; # NOTE: try both!
        "search.quickOpen.history.filterSortOrder" = "recency";

        # Per-language UI settings:
        "[css]" = { "editor.colorDecorators" = true; };

        # Files:
        "files.associations" = {
          ".babelrc" = "jsonc";
          ".eslintrc" = "jsonc";
          "*.pyi" = "python";
          "*.bats" = "shellscript";
        };
        "files.autoSave" = "onFocusChange";
        "files.exclude" = {
          "**/.DS_Store" = true;

          "**/node_modules" = true;
          "**/bower_components" = true;

          "**/__pycache__" = true;
          "**/.cache" = true;
          "**/.idea" = true;
          "**/.pytest_cache" = true;
          "**/.mypy_cache" = true;
          "**/.venv" = true;
          "**/.coverage" = true;
          "**/*.py[co]" = true;
          "**/htmlcoverage" = true;
          "**/docs/build" = true;
        };
        "workbench.editorAssociations" = { "*.ipynb" = "jupyter-notebook"; };

        # Stats and telemetry:
        "telemetry.telemetryLevel" = "off";

        # Terminal:
        "terminal.integrated.defaultLocation" = "editor";
        "terminal.integrated.shellIntegration.enabled" = false;
        "terminal.integrated.shellIntegration.decorationsEnabled" = "never";
        "terminal.integrated.stickyScroll.enabled" = false;
        "terminal.integrated.suggest.enabled" = false;
        "terminal.integrated.enableMultiLinePasteWarning" = "never";
        "terminal.integrated.defaultProfile.osx" = "zsh";
        "terminal.integrated.defaultProfile.linux" = "bash";
        "terminal.integrated.env.osx" = { "SOBOLE_THEME_MODE" = "dark"; };
        "accessibility.signals.terminalBell" = {
          "sound" = "off";
          "announcement" = "off";
        };
        "terminal.integrated.enableVisualBell" = true;
        "terminal.integrated.fontLigatures.enabled" = false;
        "terminal.integrated.sendKeybindingsToShell" = true;

        # Main editor:
        "window.clickThroughInactive" = false;
        "window.newWindowDimensions" = "maximized";
        "workbench.editor.labelFormat" = "medium";
        "workbench.startupEditor" = "newUntitledFile";
        "workbench.statusBar.visible" = false;
        "workbench.tips.enabled" = false;
        "workbench.tree.renderIndentGuides" = "none";
        "workbench.tree.enableStickyScroll" = false;
        "workbench.editor.highlightModifiedTabs" = false;
        "workbench.editor.showIcons" = false;
        "workbench.editor.showTabs" = "single";
        "workbench.editor.decorations.badges" = false;
        "workbench.editor.decorations.colors" = false;
        "workbench.editor.enablePreview" = false;
        "workbench.sideBar.location" = "right";
        "workbench.editor.empty.hint" = "hidden";
        "workbench.layoutControl.enabled" = false;

        "merge-conflict.autoNavigateNextConflict.enabled" = false;

        # Tools, extensions, and languages:
        "problems.decorations.enabled" = false;

        "git.autofetch" = false;
        "git.showInlineOpenFileAction" = false;
        "git.autorefresh" = true;
        "git.decorations.enabled" = true;
        "git.openRepositoryInParentFolders" = "never";
        "git.blame.editorDecoration.enabled" = false;

        "python.analysis.completeFunctionParens" = false;
        "python.createEnvironment.trigger" = "off";
        "python.terminal.activateEnvInCurrentTerminal" = false;
        "python.terminal.activateEnvironment" = false;
        "python.terminal.shellIntegration.enabled" = false;
        "python.languageServer" = "Pylance";
        "python.analysis.reportExtraTelemetry" = false;
        "python.analysis.addHoverSummaries" = false;
        "python.analysis.ignore" = [ "**/*.py" ]; # do not run typechecking
        "python.analysis.typeCheckingMode" = "off";
        "python.analysis.autoImportCompletions" = false;
        "python.analysis.autoSearchPaths" = false;
        "python.experiments.enabled" = false;
        "python.testing.autoTestDiscoverOnSaveEnabled" = false;

        "typescript.updateImportsOnFileMove.enabled" = "never";
        "typescript.suggest.autoImports" = false;
        "javascript.suggest.autoImports" = false;
        "javascript.updateImportsOnFileMove.enabled" = "never";

        "C_Cpp.dimInactiveRegions" = false;

        # Theme:
        # "workbench.colorTheme" = "pustota";
        "workbench.iconTheme" = "ayu";

        "editor.semanticHighlighting.enabled" = false;
        "workbench.colorCustomizations" = {
          # I customize these two, because other people might
          # want to have error and warning decorations even in `pustota` theme.
          "editorError.foreground" = "#00000000";
          "editorWarning.foreground" = "#00000000";
        };
        "editor.unicodeHighlight.nonBasicASCII" = false;

        # https://github.com/subframe7536/vscode-custom-ui-style extension:
        "custom-ui-style.electron" = {
          "frame" = false; # removes the title bar
          "transparent" = true; # removes white line from the top
          "titleBarStyle" = "hidden";
          "titleBarOverlay" = {
            "height" =
              35; # icons and text has the same centring in the title bar
          };
        };
        "window.titleBarStyle" = "native";
        "window.customTitleBarVisibility" = "never";
        # Hide unwanted elements from the UI:
        "custom-ui-style.stylesheet" = {
          # Hide extra items:
          ".monaco-toolbar, .quick-input-list .scrollbar, .quick-input-list .action-label" =
            {
              "visiblity" = "hidden !important";
              "display" = "none !important";
            };
          # Center the title with the filename:
          ".monaco-workbench .editor-group-container>.title>.label-container>.title-label>.monaco-icon-label-container" =
            {
              "margin-left" = "50px !important";
            };
          # Offset terminal title:
          ".composite.title.has-composite-bar" = {
            "margin-left" = "50px !important";
          };
        };
      };
      keybindings = [
        {
          key = "alt+left";
          command = "workbench.action.navigateBack";
        }
        {
          key = "ctrl+-";
          command = "-workbench.action.navigateBack";
        }
        {
          key = "alt+right";
          command = "workbench.action.navigateForward";
        }
        {
          key = "ctrl+shift+-";
          command = "-workbench.action.navigateForward";
        }
        {
          key = "cmd+,";
          command = "workbench.action.openSettingsJson";
        }
        {
          key = "cmd+,";
          command = "-workbench.action.openSettings";
        }
        {
          key = "cmd+e";
          command = "-actions.findWithSelection";
        }
        {
          key = "cmd+e";
          command = "editor.action.showHover";
          when = "editorTextFocus";
        }
        {
          key = "shift+enter";
          command = "-python.execSelectionInTerminal";
          when =
            "editorTextFocus && !findInputFocussed && !python.datascience.ownsSelection && !replaceInputFocussed && editorLangId == 'python'";
        }
        {
          key = "cmd+l";
          command = "copyRelativeFilePath";
        }
        {
          key = "shift+cmd+r";
          command = "-workbench.action.showAllSymbols";
        }
        {
          key = "cmd+[";
          command = "-editor.action.outdentLines";
          when = "editorTextFocus && !editorReadonly";
        }
        {
          key = "cmd+]";
          command = "-editor.action.indentLines";
          when = "editorTextFocus && !editorReadonly";
        }
        {
          key = "cmd+k cmd+\\";
          command = "-workbench.action.splitEditorOrthogonal";
        }
        {
          key = "cmd+\\";
          command = "-workbench.action.terminal.split";
          when = "terminalFocus && terminalProcessSupported";
        }
        {
          key = "cmd+p";
          command = "-workbench.action.quickOpenPreviousEditor";
        }
        {
          key = "cmd+o";
          command = "-workbench.action.files.openFile";
          when = "false";
        }
        {
          key = "cmd+o";
          command = "-workbench.action.files.openFileFolder";
          when = "isMacNative && openFolderWorkspaceSupport";
        }
        {
          key = "cmd+o";
          command = "-workbench.action.files.openLocalFileFolder";
          when = "remoteFileDialogVisible";
        }
        {
          key = "cmd+f";
          command = "-list.find";
          when = "listFocus && listSupportsFind";
        }
        {
          key = "cmd+w";
          command = "-workbench.action.terminal.killEditor";
          when =
            "terminalEditorFocus && terminalFocus && terminalHasBeenCreated || terminalEditorFocus && terminalFocus && terminalProcessSupported";
        }
        {
          key = "cmd+w";
          command = "workbench.action.terminal.kill";
          when = "terminalEditorFocus || terminalFocus";
        }
        {
          key = "cmd+f";
          command = "search.action.cancel";
          when =
            "listFocus && searchViewletVisible && !inputFocus && !treestickyScrollFocused && searchState != '0'";
        }
        {
          key = "escape";
          command = "-search.action.cancel";
          when =
            "listFocus && searchViewletVisible && !inputFocus && !treestickyScrollFocused && searchState != '0'";
        }
        {
          key = "shift+enter";
          command = "-python.execSelectionInTerminal";
          when =
            "editorTextFocus && !findInputFocussed && !isCompositeNotebook && !jupyter.ownsSelection && !notebookEditorFocused && !replaceInputFocussed && editorLangId == 'python'";
        }
        {
          key = "shift+enter";
          command = "-python.execInREPL";
          when =
            "config.python.REPL.sendToNativeREPL && editorTextFocus && !isCompositeNotebook && !jupyter.ownsSelection && !notebookEditorFocused && editorLangId == 'python'";
        }
        {
          key = "cmd+k cmd+i";
          command = "-list.showHover";
          when = "listFocus && !inputFocus && !treestickyScrollFocused";
        }
        {
          key = "cmd+i";
          command = "-toggleSuggestionDetails";
          when =
            "suggestWidgetHasFocusedSuggestion && suggestWidgetVisible && textInputFocus";
        }
        {
          key = "shift+cmd+t";
          command = "editor.action.goToTypeDefinition";
        }
        {
          key = "ctrl+i";
          command = "editor.action.triggerSuggest";
          when =
            "editorHasCompletionItemProvider && textInputFocus && !editorReadonly && !suggestWidgetVisible";
        }
        {
          key = "cmd+i";
          command = "-editor.action.triggerSuggest";
          when =
            "editorHasCompletionItemProvider && textInputFocus && !editorReadonly && !suggestWidgetVisible";
        }
        {
          key = "ctrl+f";
          command = "workbench.action.quickTextSearch";
        }
        {
          key = "ctrl+f";
          command = "-cursorRight";
          when = "textInputFocus";
        }
        {
          key = "shift+cmd+t";
          command = "workbench.action.toggleMaximizedPanel";
        }
        {
          key = "ctrl+shift+g";
          command = "-workbench.view.scm";
          when = "workbench.scm.active";
        }
        {
          key = "cmd+i";
          command = "-workbench.action.chat.startVoiceChat";
          when =
            "chatIsEnabled && hasSpeechProvider && inChatInput && !chatSessionRequestInProgress && !editorFocus && !notebookEditorFocused && !scopedVoiceChatGettingReady && !speechToTextInProgress || chatIsEnabled && hasSpeechProvider && inlineChatFocused && !chatSessionRequestInProgress && !editorFocus && !notebookEditorFocused && !scopedVoiceChatGettingReady && !speechToTextInProgress";
        }
        {
          key = "ctrl+cmd+i";
          command = "-workbench.action.chat.open";
          when = "!chatSetupHidden";
        }
        {
          key = "shift+cmd+i";
          command = "-workbench.action.chat.openAgent";
          when = "config.chat.agent.enabled && !chatSetupHidden";
        }
        {
          key = "cmd+i";
          command = "-workbench.action.chat.stopListeningAndSubmit";
          when =
            "inChatInput && voiceChatInProgress && scopedVoiceChatInProgress == 'editor' || inChatInput && voiceChatInProgress && scopedVoiceChatInProgress == 'inline' || inChatInput && voiceChatInProgress && scopedVoiceChatInProgress == 'quick' || inChatInput && voiceChatInProgress && scopedVoiceChatInProgress == 'view' || inlineChatFocused && voiceChatInProgress && scopedVoiceChatInProgress == 'editor' || inlineChatFocused && voiceChatInProgress && scopedVoiceChatInProgress == 'inline' || inlineChatFocused && voiceChatInProgress && scopedVoiceChatInProgress == 'quick' || inlineChatFocused && voiceChatInProgress && scopedVoiceChatInProgress == 'view'";
        }
        {
          key = "cmd+i";
          command = "-inlineChat2.close";
          when =
            "inlineChatHasEditsAgent && inlineChatVisible && !chatEdits.isRequestInProgress && chatEdits.requestCount == '0' || inlineChatHasEditsAgent && inlineChatVisible && !chatEdits.hasEditorModifications && !chatEdits.isRequestInProgress && chatEdits.requestCount == '0'";
        }
        {
          key = "cmd+i";
          command = "-inlineChat.start";
          when =
            "editorFocus && inlineChatHasEditsAgent && inlineChatPossible && !editorReadonly && !editorSimpleInput || editorFocus && inlineChatHasProvider && inlineChatPossible && !editorReadonly && !editorSimpleInput";
        }
        {
          key = "cmd+k i";
          command = "-inlineChat.startWithCurrentLine";
          when =
            "inlineChatHasProvider && !editorReadonly && !inlineChatVisible";
        }
        {
          key = "cmd+i";
          command = "-inlineChat.startWithCurrentLine";
          when =
            "inlineChatHasProvider && inlineChatShowingHint && !editorReadonly && !inlineChatVisible";
        }
        {
          key = "cmd+i";
          command = "-workbench.action.terminal.chat.start";
          when =
            "chatIsEnabled && terminalChatAgentRegistered && terminalFocusInAny && terminalHasBeenCreated || chatIsEnabled && terminalChatAgentRegistered && terminalFocusInAny && terminalProcessSupported";
        }
        {
          key = "cmd+i";
          command = "-inlineChat2.reveal";
          when =
            "inlineChatHasEditsAgent && !chatEdits.isGlobalEditingSession && chatEdits.requestCount >= 1";
        }
        {
          key = "shift+cmd+p shift+cmd+o";
          command = "inlineChat2.close";
          when =
            "inlineChatHasEditsAgent && inlineChatVisible && !chatEdits.isRequestInProgress && chatEdits.requestCount == '0' || inlineChatHasEditsAgent && inlineChatVisible && !chatEdits.hasEditorModifications && !chatEdits.isRequestInProgress && chatEdits.requestCount == '0'";
        }
        {
          key = "ctrl+cmd+i";
          command = "-workbench.panel.chat";
          when = "workbench.panel.chat.view.copilot.active";
        }
        {
          key = "shift+alt+up";
          command = "-editor.action.copyLinesUpAction";
          when = "editorTextFocus && !editorReadonly";
        }
        {
          key = "alt+up";
          command = "-editor.action.moveLinesUpAction";
          when = "editorTextFocus && !editorReadonly";
        }
        {
          key = "shift+alt+down";
          command = "-editor.action.copyLinesDownAction";
          when = "editorTextFocus && !editorReadonly";
        }
        {
          key = "alt+down";
          command = "-editor.action.moveLinesDownAction";
          when = "editorTextFocus && !editorReadonly";
        }
        {
          key = "ctrl+cmd+c";
          command = "editor.action.clipboardCopyWithSyntaxHighlightingAction";
        }
        {
          key = "shift+cmd+t";
          command = "workbench.action.terminal.new";
          when =
            "terminalProcessSupported || terminalWebExtensionContributedProfile";
        }
        {
          key = "ctrl+shift+`";
          command = "-workbench.action.terminal.new";
          when =
            "terminalProcessSupported || terminalWebExtensionContributedProfile";
        }
        {
          key = "ctrl+t";
          command = "-editor.action.transposeLetters";
          when = "textInputFocus && !editorReadonly";
        }
        {
          key = "alt+cmd+[";
          command = "editor.fold";
          when = "editorTextFocus";
        }
        {
          key = "alt+cmd+[";
          command = "-editor.fold";
          when = "editorTextFocus && foldingEnabled";
        }
        {
          key = "alt+cmd+]";
          command = "editor.unfold";
          when = "editorTextFocus";
        }
        {
          key = "alt+cmd+]";
          command = "-editor.unfold";
          when = "editorTextFocus && foldingEnabled";
        }
        {
          key = "shift+cmd+n";
          command = "-workbench.action.newWindow";
        }
        {
          key = "shift+cmd+n";
          command = "explorer.newFolder";
        }
        {
          key = "ctrl+shift+tab";
          command = "-workbench.action.quickOpenLeastRecentlyUsedEditorInGroup";
          when = "!activeEditorGroupEmpty";
        }
        {
          key = "cmd+d";
          command =
            "workbench.action.quickOpenPreviousRecentlyUsedEditorInGroup";
          when = "!activeEditorGroupEmpty";
        }
        {
          key = "ctrl+tab";
          command =
            "-workbench.action.quickOpenPreviousRecentlyUsedEditorInGroup";
          when = "!activeEditorGroupEmpty";
        }
        {
          key = "cmd+s";
          command = "-workbench.action.files.save";
        }
        {
          key = "cmd+down";
          command = "-editor.action.goToBottomHover";
          when = "editorHoverFocused";
        }
        {
          key = "cmd+down";
          command = "editor.action.goToBottomHover";
          when = "editorHoverFocused";
        }
        {
          key = "end";
          command = "-editor.action.goToBottomHover";
          when = "editorHoverFocused";
        }
        {
          key = "cmd+down";
          command = "-cursorBottom";
          when = "textInputFocus";
        }
        {
          key = "alt+down";
          command = "cursorBottom";
          when = "textInputFocus";
        }
        {
          key = "alt+up";
          command = "cursorTop";
          when = "textInputFocus";
        }
        {
          key = "cmd+up";
          command = "-cursorTop";
          when = "textInputFocus";
        }
        {
          key = "cmd+w";
          command = "workbench.action.terminal.killEditor";
          when = "terminalEditorFocus";
        }
        {
          key = "cmd+s";
          command = "workbench.action.files.save";
        }
        {
          key = "ctrl+left";
          command = "cursorWordLeft";
          when = "textInputFocus";
        }
        {
          key = "alt+left";
          command = "-cursorWordLeft";
          when = "textInputFocus";
        }
        {
          key = "ctrl+right";
          command = "cursorWordRight";
        }
        {
          key = "up";
          command = "cursorUp";
          when = "textInputFocus";
        }
        {
          key = "up";
          command = "-cursorUp";
          when = "textInputFocus";
        }
        {
          key = "ctrl+up";
          command = "cursorUp";
          when = "textInputFocus";
        }
        {
          key = "ctrl+down";
          command = "cursorDown";
          when = "textInputFocus";
        }
        {
          key = "cmd+k shift+alt+cmd+c";
          command = "-copyRelativeFilePath";
          when = "editorFocus";
        }
        {
          key = "shift+cmd+o";
          command = "outline.focus";
        }
        {
          key = "ctrl+d";
          command = "-deleteRight";
          when = "textInputFocus";
        }
        {
          key = "ctrl+1";
          command = "-workbench.action.openEditorAtIndex1";
        }
        {
          key = "ctrl+2";
          command = "-workbench.action.openEditorAtIndex2";
        }
        {
          key = "ctrl+3";
          command = "-workbench.action.openEditorAtIndex3";
        }
        {
          key = "ctrl+4";
          command = "-workbench.action.openEditorAtIndex4";
        }
        {
          key = "ctrl+5";
          command = "-workbench.action.openEditorAtIndex5";
        }
        {
          key = "ctrl+shift+5";
          command = "-workbench.action.terminal.split";
          when =
            "terminalFocus && terminalProcessSupported || terminalFocus && terminalWebExtensionContributedProfile";
        }
        {
          key = "ctrl+shift+5";
          command = "-workbench.action.terminal.splitActiveTab";
          when = "terminalProcessSupported && terminalTabsFocus";
        }
        {
          key = "ctrl+6";
          command = "-workbench.action.openEditorAtIndex6";
        }
        {
          key = "ctrl+7";
          command = "-workbench.action.openEditorAtIndex7";
        }
        {
          key = "ctrl+8";
          command = "-workbench.action.openEditorAtIndex8";
        }
        {
          key = "ctrl+9";
          command = "-workbench.action.openEditorAtIndex9";
        }
        {
          key = "cmd+enter";
          command = "-breadcrumbs.revealFocusedFromTreeAside";
          when =
            "breadcrumbsActive && breadcrumbsVisible && listFocus && !inputFocus && !treestickyScrollFocused";
        }
        {
          key = "cmd+k cmd+\\";
          command = "-workbench.action.splitEditorDown";
        }
        {
          key = "cmd+k shift+cmd+\\";
          command = "-workbench.action.splitEditorInGroup";
          when = "activeEditorCanSplitInGroup";
        }
        {
          key = "cmd+k cmd+\\";
          command = "-workbench.action.splitEditorLeft";
        }
        {
          key = "cmd+k cmd+\\";
          command = "-workbench.action.splitEditorUp";
        }
        {
          key = "alt+v";
          command = "workbench.action.splitEditorRight";
        }
        {
          key = "cmd+k cmd+\\";
          command = "-workbench.action.splitEditorRight";
        }
        {
          key = "ctrl+enter";
          command = "-explorer.openToSide";
          when = "explorerViewletFocus && foldersViewVisible && !inputFocus";
        }
        {
          key = "ctrl+enter";
          command = "-search.action.openResultToSide";
          when = "fileMatchOrMatchFocus && searchViewletVisible";
        }
        {
          key = "ctrl+enter";
          command = "-openReferenceToSide";
          when =
            "listFocus && referenceSearchVisible && !inputFocus && !treeElementCanCollapse && !treeElementCanExpand && !treestickyScrollFocused";
        }
      ];
      extensions = with pkgs.vscode-extensions; [
        bbenoist.nix
        jnoortheen.nix-ide

        ms-python.python
        ms-python.vscode-pylance

        golang.go

        pkief.material-icon-theme
        teabyii.ayu
        vscodevim.vim
      ];
    };
  };
}
