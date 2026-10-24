#!/usr/bin/env bash
# Check installed versions, or reinstall the reviewed pins. No Git/model actions.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [[ $# -eq 0 ]]; then exec bash "$root/scripts/jcode-install.sh"; fi
if [[ $# -ne 1 || $1 != --check ]]; then echo 'Usage: scripts/jcode-update.sh [--check]' >&2; exit 2; fi
data=${JCODE_INTEGRATIONS_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/jcode-integrations}
node=${JCODE_NODE_BIN:-$(command -v node)}
"$node" --input-type=module - "$root" "$data" <<'JS'
import fs from 'node:fs';
import path from 'node:path';
const [root, data] = process.argv.slice(2);
const wanted = JSON.parse(fs.readFileSync(path.join(root, 'config/jcode/package.json'))).dependencies;
for (const [name, version] of Object.entries(wanted)) {
  const file = path.join(data, 'runtime/node_modules', name, 'package.json');
  const installed = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file)).version : 'missing';
  console.log(`${name}: ${installed} (pin ${version})`);
  if (installed !== version) process.exitCode = 1;
}
JS
while IFS= read -r spec; do
  [[ -z "$spec" || "$spec" == \#* ]] && continue
  name=${spec%%==*}
  "$data/python/$name/bin/python" -B - "$name" "${spec#*==}" <<'PY'
import importlib.metadata, sys
name, wanted = sys.argv[1:]
installed = importlib.metadata.version(name)
print(f'{name}: {installed} (pin {wanted})')
sys.exit(0 if installed == wanted else 1)
PY
done < "$root/config/jcode/python-tools.txt"
docker inspect jcode-neo4j --format 'Neo4j: {{.State.Status}} / {{if .State.Health}}{{.State.Health.Status}}{{end}}'
