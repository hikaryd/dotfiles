eval "$(/opt/homebrew/bin/brew shellenv)"

# malt — ставится через Maltfile, prefix /opt/malt. Кладём в PATH перед brew,
# но не перетираем HOMEBREW_* (нужны brew для cask).
if [ -x /opt/malt/bin/malt ]; then
  export PATH="/opt/malt/bin:/opt/malt/sbin:$PATH"
  export MANPATH="/opt/malt/share/man:${MANPATH:-}"
  export INFOPATH="/opt/malt/share/info:${INFOPATH:-}"
fi
