# Ayu dark theme for Nushell
# Generated from palette/ayu.toml

let theme = {
  red: "#f07178"
  green: "#aad94c"
  yellow: "#ffb454"
  blue: "#39bae6"
  magenta: "#d2a6ff"
  cyan: "#95e6cb"
  white: "#e6e1cf"
  black: "#3e4451"
  text: "#bfbdb6"
  accent: "#e6b450"
  dim: "#555e73"
  line: "#1b1f29"
  bg: "#0d1017"
  tag: "#39bae6"
  func: "#ffb454"
  entity: "#59c2ff"
  string: "#aad94c"
  regexp: "#95e6cb"
  markup: "#f07178"
  keyword: "#ff8f40"
  special: "#e6c08a"
  comment: "#546178"
  constant: "#d2a6ff"
  operator: "#f29668"
  error: "#d95757"
}

let scheme = {
  recognized_command: $theme.entity
  unrecognized_command: $theme.text
  constant: $theme.constant
  punctuation: $theme.dim
  operator: $theme.operator
  string: $theme.string
  virtual_text: $theme.comment
  variable: { fg: $theme.special attr: i }
  filepath: $theme.func
}

$env.config.color_config = {
  separator: { fg: $theme.dim attr: b }
  leading_trailing_space_bg: { fg: $theme.accent attr: u }
  header: { fg: $theme.text attr: b }
  row_index: $scheme.virtual_text
  record: $theme.text
  list: $theme.text
  hints: $scheme.virtual_text
  search_result: { fg: $theme.bg bg: $theme.accent }
  shape_closure: $theme.regexp
  closure: $theme.regexp
  shape_flag: { fg: $theme.markup attr: i }
  shape_matching_brackets: { attr: u }
  shape_garbage: $theme.red
  shape_keyword: $theme.keyword
  shape_match_pattern: $theme.string
  shape_signature: $theme.regexp
  shape_table: $scheme.punctuation
  cell-path: $scheme.punctuation
  shape_list: $scheme.punctuation
  shape_record: $scheme.punctuation
  shape_vardecl: $scheme.variable
  shape_variable: $scheme.variable
  empty: { attr: n }
  filesize: {||
    if $in < 1kb {
      $theme.cyan
    } else if $in < 10kb {
      $theme.green
    } else if $in < 100kb {
      $theme.yellow
    } else if $in < 10mb {
      $theme.func
    } else if $in < 100mb {
      $theme.markup
    } else if $in < 1gb {
      $theme.red
    } else {
      $theme.keyword
    }
  }
  duration: {||
    if $in < 1day {
      $theme.cyan
    } else if $in < 1wk {
      $theme.green
    } else if $in < 4wk {
      $theme.yellow
    } else if $in < 12wk {
      $theme.func
    } else if $in < 24wk {
      $theme.markup
    } else if $in < 52wk {
      $theme.red
    } else {
      $theme.keyword
    }
  }
  date: {|| (date now) - $in |
    if $in < 1day {
      $theme.cyan
    } else if $in < 1wk {
      $theme.green
    } else if $in < 4wk {
      $theme.yellow
    } else if $in < 12wk {
      $theme.func
    } else if $in < 24wk {
      $theme.markup
    } else if $in < 52wk {
      $theme.red
    } else {
      $theme.keyword
    }
  }
  shape_external: $scheme.unrecognized_command
  shape_internalcall: $scheme.recognized_command
  shape_external_resolved: $scheme.recognized_command
  shape_block: $scheme.recognized_command
  block: $scheme.recognized_command
  shape_custom: $theme.tag
  custom: $theme.tag
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
  string: $scheme.string
  shape_string: $scheme.string
  shape_string_interpolation: $theme.operator
  shape_raw_string: $scheme.string
  shape_externalarg: $scheme.string
}
$env.config.highlight_resolved_externals = true
$env.config.explore = {
    status_bar_background: { fg: $theme.text, bg: $theme.bg },
    command_bar_text: { fg: $theme.text },
    highlight: { fg: $theme.bg, bg: $theme.accent },
    status: {
        error: $theme.red,
        warn: $theme.yellow,
        info: $theme.blue,
    },
    selected_cell: { bg: $theme.entity fg: $theme.bg },
}
