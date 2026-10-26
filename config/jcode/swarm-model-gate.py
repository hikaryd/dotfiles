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
if not isinstance(model, str) or not model.strip() or model.strip().lower() in {'inherit', 'coordinator'}:
    print(
        'Choose an explicit model and effort for this delegation and retry. '
        'Use x5-copilot:x5-airun-code-large with effort max for well-specified work, '
        'openai-oauth:gpt-6.1-sol with effort low for paid work, '
        'openai-oauth:gpt-6.1-sol with effort xhigh for review, '
        'or openai-oauth:gpt-6.1-terra with effort low when appropriate. '
        'A reused worker keeps its current model and effort.',
        file=sys.stderr,
    )
    sys.exit(2)

label = payload.get('label')
if action == 'spawn' and isinstance(label, str) and label.strip().lower() == 'reviewer':
    if model.strip() != 'openai-oauth:gpt-6.1-sol' or payload.get('effort') != 'xhigh':
        print('Use openai-oauth:gpt-6.1-sol with effort xhigh for reviewer and retry.', file=sys.stderr)
        sys.exit(2)
    sys.exit(0)

expected = 'max' if model.strip() == 'x5-copilot:x5-airun-code-large' else 'low'
if payload.get('effort') != expected:
    print(f'Use effort "{expected}" for this worker and retry. A reused worker keeps its current effort.', file=sys.stderr)
    sys.exit(2)
