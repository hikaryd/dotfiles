/// <reference path="./lib/fresh.d.ts" />
/**
 * Auto-enable Vi mode + custom Neovim-style leader keybindings.
 *
 * This plugin:
 * 1. Waits for the embedded vi_mode plugin to load
 * 2. Redefines vi-normal mode adding Space-leader combos and overrides
 * 3. Auto-enables vi mode on startup
 */
const editor = getEditor();

// ── Leader-key actions ──────────────────────────────────────────────
globalThis.leader_quit = function (): void {
  editor.executeAction("close_buffer");
};
globalThis.leader_file_explorer = function (): void {
  editor.executeAction("toggle_file_explorer");
};
globalThis.leader_vsplit = function (): void {
  editor.executeAction("split_vertical");
};
globalThis.leader_prev_split = function (): void {
  editor.executeAction("prev_split");
};
globalThis.leader_next_split = function (): void {
  editor.executeAction("next_split");
};
globalThis.leader_open_file = function (): void {
  editor.executeAction("open");
};
globalThis.leader_search = function (): void {
  editor.executeAction("search");
};
globalThis.leader_diagnostics = function (): void {
  editor.executeAction("diagnostics");
};
globalThis.leader_prev_buffer = function (): void {
  editor.executeAction("prev_buffer");
};
globalThis.leader_next_buffer = function (): void {
  editor.executeAction("next_buffer");
};
globalThis.leader_save = function (): void {
  editor.executeAction("save");
};

// ── LSP actions (g-prefix and K) ────────────────────────────────────
globalThis.vi_goto_definition = function (): void {
  editor.executeAction("lsp_goto_definition");
};
globalThis.vi_goto_references = function (): void {
  editor.executeAction("lsp_references");
};
globalThis.vi_hover = function (): void {
  editor.executeAction("lsp_hover");
};
globalThis.vi_code_actions = function (): void {
  editor.executeAction("lsp_code_actions");
};

// ── Extra actions ───────────────────────────────────────────────────
globalThis.vi_move_line_down = function (): void {
  editor.executeAction("move_line_down");
};
globalThis.vi_move_line_up = function (): void {
  editor.executeAction("move_line_up");
};
globalThis.vi_select_all = function (): void {
  editor.executeAction("select_all");
};

// ── Redefine vi-normal with all original + custom bindings ──────────
globalThis.onEditorReady = function (): void {
  // Enable vi mode via the embedded plugin's global function
  if (typeof globalThis.vi_mode_toggle === "function") {
    globalThis.vi_mode_toggle();
  }

  // Redefine vi-normal mode: original bindings + leader keys + LSP
  editor.defineMode("vi-normal", null, [
    // ── Count prefix ──
    ["1", "vi_digit_1"],
    ["2", "vi_digit_2"],
    ["3", "vi_digit_3"],
    ["4", "vi_digit_4"],
    ["5", "vi_digit_5"],
    ["6", "vi_digit_6"],
    ["7", "vi_digit_7"],
    ["8", "vi_digit_8"],
    ["9", "vi_digit_9"],
    ["0", "vi_digit_0_or_line_start"],

    // ── Navigation ──
    ["h", "vi_left"],
    ["j", "vi_down"],
    ["k", "vi_up"],
    ["l", "vi_right"],
    ["w", "vi_word"],
    ["b", "vi_word_back"],
    ["e", "vi_word_end"],
    ["$", "vi_line_end"],
    ["^", "vi_first_non_blank"],
    ["g g", "vi_doc_start"],
    ["G", "vi_doc_end"],
    ["C-f", "vi_page_down"],
    ["C-b", "vi_page_up"],
    ["C-d", "vi_half_page_down"],
    ["C-u", "vi_half_page_up"],
    ["%", "vi_matching_bracket"],
    ["z z", "vi_center_cursor"],

    // ── Search ──
    ["/", "vi_search_forward"],
    ["?", "vi_search_backward"],
    ["n", "vi_find_next"],
    ["N", "vi_find_prev"],

    // ── Find character ──
    ["f", "vi_find_char_f"],
    ["t", "vi_find_char_t"],
    ["F", "vi_find_char_F"],
    ["T", "vi_find_char_T"],
    // NOTE: ; remapped to command_palette (use , for find-char repeat)
    [",", "vi_find_char_repeat_reverse"],

    // ── Mode switching ──
    ["i", "vi_insert_before"],
    ["a", "vi_insert_after"],
    ["I", "vi_insert_line_start"],
    ["A", "vi_insert_line_end"],
    ["o", "vi_open_below"],
    ["O", "vi_open_above"],
    ["Escape", "vi_escape"],

    // ── Operators ──
    ["d", "vi_delete_operator"],
    ["c", "vi_change_operator"],
    ["y", "vi_yank_operator"],

    // ── Single char operations ──
    ["x", "vi_delete_char"],
    ["X", "vi_delete_char_before"],
    ["r", "vi_replace_char"],
    ["s", "vi_substitute"],
    ["S", "vi_change_line"],
    ["D", "vi_delete_to_end"],
    ["C", "vi_change_to_end"],

    // ── Clipboard ──
    ["p", "vi_paste_after"],
    ["P", "vi_paste_before"],

    // ── Undo/Redo ──
    ["u", "vi_undo"],
    ["C-r", "vi_redo"],

    // ── Repeat ──
    [".", "vi_repeat"],

    // ── Visual mode ──
    ["v", "vi_visual_char"],
    ["V", "vi_visual_line"],
    ["C-v", "vi_visual_block"],

    // ── Other ──
    ["J", "vi_join"],
    [":", "vi_command_mode"],
    ["C-p", "command_palette"],
    ["C-q", "quit"],

    // ════════════════════════════════════════════════════════════════
    // CUSTOM: Neovim-style overrides
    // ════════════════════════════════════════════════════════════════

    // ;  → command palette  (vim: ; was mapped to : in Neovim)
    [";", "command_palette"],

    // K  → LSP hover docs  (like Neovim K → Lspsaga hover_doc)
    ["K", "vi_hover"],

    // gd → go to definition  (like Neovim gd)
    ["g d", "vi_goto_definition"],
    // gr → find references  (like Neovim gr)
    ["g r", "vi_goto_references"],

    // Ctrl+S → save  (like Neovim <C-s>)
    ["C-s", "leader_save"],

    // Alt+j/k → move lines  (like Neovim <A-j>/<A-k>)
    ["A-j", "vi_move_line_down"],
    ["A-k", "vi_move_line_up"],

    // ── Space-leader combos ──
    ["Space q", "leader_quit"],
    ["Space e", "leader_file_explorer"],
    ["Space v", "leader_vsplit"],
    ["Space h", "leader_prev_split"],
    ["Space l", "leader_next_split"],
    ["Space Space", "leader_open_file"],
    ["Space /", "leader_search"],
    ["Space d", "leader_diagnostics"],
    ["Space b", "leader_prev_buffer"],
    ["Space n", "leader_next_buffer"],
  ], true);

  // Also add Alt+j/k to vi-insert mode
  editor.defineMode("vi-insert", null, [
    ["Escape", "vi_escape"],
    ["C-p", "command_palette"],
    ["C-q", "quit"],
    ["C-s", "leader_save"],
    ["A-j", "vi_move_line_down"],
    ["A-k", "vi_move_line_up"],
  ], false);
};

editor.on("editor_initialized", "onEditorReady");
