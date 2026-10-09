# Quiet Ayu mirage для Nushell
# Сгенерировано из palette/ayu.toml; цвета ввода согласованы с zsh.

let theme = {
  red: "#f28779"
  green: "#d5ff80"
  yellow: "#ffd173"
  blue: "#5ccfe6"
  magenta: "#dfbfff"
  cyan: "#95e6cb"
  white: "#e6e1cf"
  black: "#3e4451"
  text: "#cccac2"
  accent: "#73d0ff"
  dim: "#707a8c"
  line: "#171b24"
  selection: "#34455a"
  warning: "#ffcc66"
  bg: "#1f2430"
  tag: "#5ccfe6"
  func: "#ffd173"
  entity: "#73d0ff"
  string: "#d5ff80"
  regexp: "#95e6cb"
  markup: "#f28779"
  keyword: "#ffad66"
  special: "#ffdfb3"
  comment: "#6e7f97"
  constant: "#dfbfff"
  operator: "#f29e74"
  error: "#ff6666"
}

let scheme = {
  recognized_command: $theme.accent
  unrecognized_command: $theme.text
  constant: $theme.text
  punctuation: $theme.dim
  operator: $theme.dim
  string: $theme.regexp
  virtual_text: $theme.dim
  variable: $theme.text
  filepath: $theme.text
}

$env.config.color_config = {
  separator: $theme.dim
  leading_trailing_space_bg: { fg: $theme.accent attr: u }
  header: { fg: $theme.text attr: b }
  row_index: $scheme.virtual_text
  record: $theme.text
  list: $theme.text
  hints: $scheme.virtual_text
  search_result: { fg: $theme.accent bg: $theme.selection }
  shape_closure: $theme.dim
  closure: $theme.dim
  shape_flag: $theme.dim
  shape_matching_brackets: { attr: u }
  shape_garbage: $theme.error
  shape_keyword: $theme.accent
  shape_match_pattern: $scheme.string
  shape_signature: $theme.dim
  shape_table: $scheme.punctuation
  cell-path: $scheme.punctuation
  shape_list: $scheme.punctuation
  shape_record: $scheme.punctuation
  shape_vardecl: $scheme.variable
  shape_variable: $scheme.variable
  empty: { attr: n }
  # Статичные цвета: без радуги и вычислений для каждой ячейки.
  filesize: $theme.text
  duration: $theme.dim
  date: $theme.dim
  shape_external: $scheme.unrecognized_command
  shape_internalcall: $scheme.recognized_command
  shape_external_resolved: $scheme.recognized_command
  shape_block: $scheme.recognized_command
  block: $scheme.recognized_command
  shape_custom: $theme.text
  custom: $theme.text
  background: $theme.bg
  foreground: $theme.text
  cursor: { bg: $theme.accent fg: $theme.bg }
  shape_range: $scheme.operator
  range: $scheme.operator
  shape_pipe: $scheme.operator
  shape_operator: $scheme.operator
  shape_redirection: $scheme.operator
  glob: $scheme.filepath
  shape_directory: $scheme.filepath
  shape_filepath: $scheme.filepath
  shape_glob_interpolation: $scheme.filepath
  shape_globpattern: $scheme.filepath
  shape_int: $scheme.constant
  int: $scheme.constant
  bool: $scheme.constant
  float: $scheme.constant
  nothing: $scheme.constant
  binary: $scheme.constant
  shape_nothing: $scheme.constant
  shape_bool: $scheme.constant
  shape_float: $scheme.constant
  shape_binary: $scheme.constant
  shape_datetime: $scheme.constant
  shape_literal: $scheme.constant
  string: $theme.text
  shape_string: $scheme.string
  shape_string_interpolation: $scheme.string
  shape_raw_string: $scheme.string
  shape_externalarg: $theme.text
}
$env.config.highlight_resolved_externals = true
$env.config.explore = {
    status_bar_background: { fg: $theme.text, bg: $theme.bg },
    command_bar_text: { fg: $theme.text },
    highlight: { fg: $theme.accent, bg: $theme.selection },
    status: {
        error: $theme.error,
        warn: $theme.warning,
        info: $theme.blue,
    },
    selected_cell: { bg: $theme.selection fg: $theme.text },
}
