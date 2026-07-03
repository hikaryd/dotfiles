#!/usr/bin/env bash
#
# Аудит «лишнего»: что установлено в системе мимо Brewfile/дотфайлов.
# Ничего не удаляет — только отчёт (кандидаты для дальнейшей чистки).
#
# Вывод компактный: списки в несколько колонок под ширину терминала;
# «*» = для пакета уже есть конфиг в dots → его надо добавить в Brewfile,
# а не удалять.
#
# Диф считается по рецептам Cellar (installed_on_request), а не через
# `brew bundle cleanup` / `brew leaves` / `brew info --json`: все трое
# молча пропускают formulae из недоверенных taps (sketchybar, rift, …).
set -uo pipefail

BASEDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BREWFILE="$BASEDIR/Brewfile"

if [[ -t 1 ]]; then B=$'\033[1m'; DIM=$'\033[2m'; R=$'\033[0m'; YEL=$'\033[33m'; else B=''; DIM=''; R=''; YEL=''; fi
TERM_W=${COLUMNS:-$(tput cols 2>/dev/null || echo 100)}
(( TERM_W > 120 )) && TERM_W=120

section() { printf '\n%s━━ %s%s\n' "$B" "$*" "$R"; }
count()   { awk 'NF{n++} END{print n+0}'; }

# stdin: элементы → ровные колонки (по вертикали) под ширину терминала
cols() {
  awk -v w="$TERM_W" '
    NF { items[n++] = $0; if (length($0) > max) max = length($0) }
    END {
      if (!n) { print "   —"; exit }
      cw = max + 2
      nc = int((w - 3) / cw); if (nc < 1) nc = 1
      nr = int((n + nc - 1) / nc)
      for (r = 0; r < nr; r++) {
        line = "   "
        for (c = 0; c < nc; c++) {
          i = c * nr + r
          if (i < n) line = line sprintf("%-" cw "s", items[i])
        }
        sub(/ +$/, "", line)
        print line
      }
    }'
}

command -v brew >/dev/null 2>&1 || { echo "brew не найден" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "нужен jq (brew install jq) — без него отчёт будет ложно-пустым" >&2; exit 1; }
[[ -f $BREWFILE ]] || { echo "нет $BREWFILE" >&2; exit 1; }

# имена без tap-префикса: "acsandmann/tap/rift" -> "rift"
base() { awk -F/ '{print $NF}'; }

# помечает «*» пакеты, у которых есть конфиг в dots (точное имя или общий
# префикс от 4 символов: xray ↔ xray-codex)
CONF_NAMES=$(ls "$BASEDIR/config" 2>/dev/null)
mark_dots() {
  local n c m
  while IFS= read -r n; do
    [[ -n $n ]] || continue
    m=""
    while IFS= read -r c; do
      [[ -n $c ]] || continue
      if [[ $n == "$c" || ( ${#n} -ge 4 && $c == "$n"* ) || ( ${#c} -ge 4 && $n == "$c"* ) ]]; then
        m="*"; break
      fi
    done <<<"$CONF_NAMES"
    printf '%s%s\n' "$n" "$m"
  done
}

# ─── сбор данных ─────────────────────────────────────────────────────
BF_FORMULAE=$(sed -n 's/^brew "\([^"]*\)".*/\1/p' "$BREWFILE" | base | sort -u)
BF_CASKS=$(sed -n 's/^cask "\([^"]*\)".*/\1/p' "$BREWFILE" | base | sort -u)
BF_TAPS=$(sed -n 's/^tap "\([^"]*\)".*/\1/p' "$BREWFILE" | sort -u)
# raycast ставится отдельным шагом install (опциональный cask) — не «лишний»
BF_CASKS=$(printf '%s\nraycast\n' "$BF_CASKS" | sort -u)

CELLAR=$(brew --cellar)
INST_FORMULAE=$(for r in "$CELLAR"/*/*/INSTALL_RECEIPT.json; do
  [[ -e $r ]] || continue
  jq -e '.installed_on_request == true' "$r" >/dev/null 2>&1 \
    && basename "$(dirname "$(dirname "$r")")"
done | sort -u)
INST_CASKS=$(brew list --cask 2>/dev/null | base | sort -u)
INST_TAPS=$(brew tap 2>/dev/null | sort -u)

X_FORMULAE=$(comm -23 <(printf '%s\n' "$INST_FORMULAE") <(printf '%s\n' "$BF_FORMULAE"))
X_CASKS=$(comm -23 <(printf '%s\n' "$INST_CASKS") <(printf '%s\n' "$BF_CASKS"))
X_TAPS=$(comm -23 <(printf '%s\n' "$INST_TAPS") <(printf '%s\n' "$BF_TAPS"))

# .app мимо brew и App Store
CASK_APPS=$(brew info --json=v2 --installed 2>/dev/null \
  | jq -r '.casks[].artifacts[]? | objects | .app[]? | strings' 2>/dev/null | base | sort -u)
norm() { tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9'; }
CASK_TOKENS_NORM=$(printf '%s\n' "$INST_CASKS" | while IFS= read -r t; do printf '%s\n' "$t" | norm; echo; done | sort -u)
X_APPS=""; MAS_APPS=""
for dir in /Applications "$HOME/Applications"; do
  [[ -d $dir ]] || continue
  for app in "$dir"/*.app; do
    [[ -e $app ]] || continue
    name=$(basename "$app" .app)
    [[ $name == Safari ]] && continue
    if [[ -e "$app/Contents/_MASReceipt/receipt" ]]; then
      MAS_APPS+="$name"$'\n'; continue
    fi
    grep -qxF "$name.app" <<<"$CASK_APPS" && continue
    grep -qxF "$(printf '%s' "$name" | norm)" <<<"$CASK_TOKENS_NORM" && continue
    [[ $dir == "$HOME"* ]] && name="$name (~)"
    X_APPS+="$name"$'\n'
  done
done
X_APPS=$(sort -f <<<"$X_APPS")
MAS_APPS=$(sort -f <<<"$MAS_APPS")

NPM_PKGS=""
command -v npm >/dev/null 2>&1 \
  && NPM_PKGS=$(npm ls -g --depth=0 2>/dev/null | tail -n +2 | sed -E 's/^[^A-Za-z0-9@]*//' | awk 'NF')

N_F=$(count <<<"$X_FORMULAE"); N_C=$(count <<<"$X_CASKS"); N_T=$(count <<<"$X_TAPS")
N_A=$(count <<<"$X_APPS");    N_M=$(count <<<"$MAS_APPS"); N_N=$(count <<<"$NPM_PKGS")

# ─── отчёт ───────────────────────────────────────────────────────────
section "Установлено мимо Brewfile — $(date +%Y-%m-%d)"
printf '   formulae %s%s%s · casks %s%s%s · taps %s%s%s · приложения %s%s%s (+%s App Store) · npm -g %s\n' \
  "$B" "$N_F" "$R" "$B" "$N_C" "$R" "$B" "$N_T" "$R" "$B" "$N_A" "$R" "$N_M" "$N_N"
printf '   %s%s*%s%s = конфиг уже в dots → пакет стоит добавить в Brewfile, а не удалять%s\n' "$DIM" "$YEL" "$R" "$DIM" "$R"

section "brew formulae ($N_F)"
mark_dots <<<"$X_FORMULAE" | cols

section "brew casks ($N_C)"
mark_dots <<<"$X_CASKS" | cols

section "brew taps ($N_T)"
cols <<<"$X_TAPS"

section "Приложения мимо brew и App Store ($N_A)"
cols <<<"$X_APPS"
printf '   %s(~) = из ~/Applications · pkg-казки могут попасть сюда ложно — сверься с brew list --cask%s\n' "$DIM" "$R"

section "App Store ($N_M)"
cols <<<"$MAS_APPS"

section "npm -g ($N_N)"
cols <<<"$NPM_PKGS"

printf '\n%sОтчёт информационный, ничего не удалено. Чистка: brew uninstall <f> · brew uninstall --cask <c> · brew untap <t>%s\n' "$DIM" "$R"
