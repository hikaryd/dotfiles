#!/usr/bin/env bash
set -euo pipefail
if [[ ${1:-} == --help && $# -eq 1 ]]; then
  printf 'Использование: ./install.sh\nPYTHON задаёт Python 3.9+ (по умолчанию python3). Пакеты не скачиваются.\n'
  exit 0
fi
[[ $# -eq 0 ]] || { printf 'Использование: ./install.sh [--help]\n' >&2; exit 2; }
[[ $(uname -s) == Darwin ]] || { printf 'Требуется macOS.\n' >&2; exit 1; }
[[ $(id -u) != 0 ]] || { printf 'Запускайте установку от обычного пользователя, не root.\n' >&2; exit 1; }
python=${PYTHON:-python3}
command -v "$python" >/dev/null || { printf 'Не найден Python: %s\n' "$python" >&2; exit 1; }
"$python" -I -c 'import sys; sys.exit(sys.version_info < (3, 9))' || { printf 'Требуется Python 3.9 или новее.\n' >&2; exit 1; }
for tool in /usr/bin/sandbox-exec /usr/bin/codesign; do
  [[ -x $tool ]] || { printf 'Не найден необходимый инструмент: %s\n' "$tool" >&2; exit 1; }
done
root=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
exec "$python" -I "$root/src/install_local.py"
