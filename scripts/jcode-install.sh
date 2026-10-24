#!/usr/bin/env bash
# Original upstream MCP tools; no adapters, fork builds, model workers or keys in Git.
set -euo pipefail
umask 077
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [[ $# -ne 0 ]]; then echo 'Usage: scripts/jcode-install.sh' >&2; exit 2; fi
for cmd in node npm uv docker go gopls; do command -v "$cmd" >/dev/null || { echo "jcode: $cmd required" >&2; exit 1; }; done
docker info >/dev/null
node=${JCODE_NODE_BIN:-$(command -v node)}
"$node" -e 'const [major, minor] = process.versions.node.split(".").map(Number); if (major < 22 || (major === 22 && minor < 19)) { console.error("Node.js >=22.19 is required"); process.exit(1); }'
npm=$(command -v npm)
export JCODE_NODE_BIN="$node"
data=${JCODE_INTEGRATIONS_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/jcode-integrations}
export JCODE_INTEGRATIONS_DIR="$data" UV_PYTHON_INSTALL_DIR="$data/python-runtime"
for dir in "$data" "$data/runtime" "$data/web" "$data/web/config" "$data/python" "$data/serena"; do
  if [[ -L "$dir" || ( -e "$dir" && ! -d "$dir" ) ]]; then echo 'jcode: expected regular integration directories' >&2; exit 1; fi
  mkdir -p "$dir"
done
chmod 700 "$data" "$data/web/config" "$data/serena"
install -m 644 "$root/config/jcode/package.json" "$root/config/jcode/package-lock.json" "$data/runtime/"
if [[ ! -e "$data/web/config/web-search.json" && ! -L "$data/web/config/web-search.json" ]]; then
  install -m 600 "$root/config/jcode/web-search.example.json" "$data/web/config/web-search.json"
fi
(cd "$data/runtime" && "$node" "$npm" ci --ignore-scripts --no-audit --no-fund)
uv python install --no-bin 3.13
python=$(uv python find --managed-python 3.13)
while IFS= read -r spec; do
  [[ -z "$spec" || "$spec" == \#* ]] && continue
  envdir="$data/python/${spec%%==*}"
  if [[ -L "$envdir" ]]; then echo 'jcode: Python environment must not be a symlink' >&2; exit 1; fi
  [[ -d "$envdir" ]] || uv venv --python "$python" "$envdir"
  uv pip install --python "$envdir/bin/python" "$spec"
done < "$root/config/jcode/python-tools.txt"
# Original health check exercises real gopls before replacing working Lens.
fixture=$(mktemp -d "${TMPDIR:-/tmp}/jcode-lsp.XXXXXXXX")
trap 'rm -rf "$fixture"' EXIT
printf 'module example.com/jcodehealth\n\ngo 1.23\n' > "$fixture/go.mod"
{
  printf 'package health\n\nfunc Greeting(name string) string { return "Hello " + name }\n'
  printf '// synthetic health fixture padding %0100d\n' {1..20}
} > "$fixture/hello.go"
SERENA_HOME="$data/serena" "$data/python/serena-agent/bin/serena" project health-check "$fixture"
"$root/scripts/build-go-tools.sh"
"$root/build/bin/dots-tools" setup jcode-neo4j-init
# No expanded compose config, password arguments or shell tracing.
compose=(docker compose --env-file "$data/neo4j/private.env" -f "$root/config/jcode/neo4j.compose.yml")
"${compose[@]}" up -d --wait --wait-timeout 180
JCODE_NEO4J_ENV="$data/neo4j/private.env" "$data/python/mcp-neo4j-cypher/bin/python" - <<'PY'
import os, pathlib, time
from neo4j import GraphDatabase
password = pathlib.Path(os.environ['JCODE_NEO4J_ENV']).read_text().strip().split('=', 1)[1]
with GraphDatabase.driver('bolt://127.0.0.1:7687', auth=('neo4j', password)) as driver:
    for attempt in range(30):
        try:
            driver.execute_query('RETURN 1 AS ready', database_='neo4j')
            break
        except Exception:
            if attempt == 29:
                raise SystemExit('jcode: Neo4j authenticated readiness failed') from None
            time.sleep(1)
print('jcode: Neo4j authenticated readiness passed')
PY
"$root/build/bin/dots-tools" setup jcode-apply
printf 'jcode: Codex-only ready MCP setup installed. Restart an existing session.\n'
