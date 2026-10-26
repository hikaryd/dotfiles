import json
import os
import sys


if os.environ.get('JCODE_HOOK_TOOL_NAME') != 'swarm':
    sys.exit(0)

try:
    payload = json.load(sys.stdin)
except (ValueError, OSError):
    sys.exit(0)

if not isinstance(payload, dict):
    sys.exit(0)

action = payload.get('action')
if isinstance(action, str) and action.strip().lower() == 'assign':
    action = 'assign_task'
if action in {'assign_task', 'assign_next', 'fill_slots'}:
    if payload.get('target_session') is not None or not (
        payload.get('prefer_spawn') is True or payload.get('spawn_if_needed') is True
    ):
        sys.exit(0)
elif action not in {'spawn', 'run_plan'}:
    sys.exit(0)

model = payload.get('model')
if isinstance(model, str) and model.strip() and model.strip().lower() not in {'inherit', 'coordinator'}:
    sys.exit(0)

print(
    'Choose an explicit model for this delegation and retry. '
    'Use x5-copilot:x5-airun-code-large with effort max for well-specified implementation, '
    'openai-oauth:gpt-6.1-sol with effort high for consequential independent review, '
    'or openai-oauth:gpt-6.1-terra for ambiguous design, diagnosis and integration. '
    'Explicit choices and user-requested models are allowed. A reused worker keeps its current model.',
    file=sys.stderr,
)
sys.exit(2)
