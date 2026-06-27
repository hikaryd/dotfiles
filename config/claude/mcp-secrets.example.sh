# shellcheck shell=bash
# Секреты для Claude Code MCP-серверов и env.
#
# Это ШАБЛОН. Скопируйте его в ~/.claude/.mcp-secrets и впишите реальные значения:
#
#   cp config/claude/mcp-secrets.example.sh ~/.claude/.mcp-secrets
#   $EDITOR ~/.claude/.mcp-secrets
#
# Реальный файл ~/.claude/.mcp-secrets живёт ВНЕ репозитория и не коммитится.
# steps/claude.yml подгружает его при установке и подставляет значения в
# settings.json и mcp-servers.json. Без него MCP/env-шаги аккуратно пропускаются.

# Z.AI MCP (@z_ai/mcp-server)
export Z_AI_API_KEY="your-zai-api-key-here"

# 21st.dev Magic MCP (@21st-dev/magic)
export MAGIC_API_KEY="your-magic-api-key-here"

# Путь к kubeconfig для kubernetes MCP
export KUBECONFIG_PATH="$HOME/.kube/configs/default/your-cluster.yaml"

# Базовый URL для Anthropic API (напр. Cloudflare AI Gateway).
# Оставьте пустым/закомментируйте, чтобы использовать дефолтный api.anthropic.com.
export ANTHROPIC_BASE_URL="https://api.anthropic.com"
