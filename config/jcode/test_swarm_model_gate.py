import json
import os
import subprocess
import sys
import unittest
from pathlib import Path


HOOK = Path(__file__).with_name('swarm-model-gate.py')


def run_hook(tool, payload):
    env = os.environ | {'JCODE_HOOK_TOOL_NAME': tool}
    return subprocess.run(
        [sys.executable, str(HOOK)],
        input=json.dumps(payload),
        text=True,
        capture_output=True,
        env=env,
        check=False,
    )


class SwarmModelGateTests(unittest.TestCase):
    def test_spawn_requires_explicit_model(self):
        for model in (None, '', 'inherit', ' INHERIT ', 'coordinator'):
            with self.subTest(model=model):
                result = run_hook('swarm', {'action': 'spawn', 'label': 'reviewer', 'model': model})
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertIn('openai-oauth:gpt-6.1-sol', result.stderr)
                self.assertIn('x5-copilot:x5-airun-code-large', result.stderr)

    def test_explicit_model_and_non_spawn_are_not_blocked(self):
        for tool, payload in (
            ('swarm', {'action': 'spawn', 'label': 'reviewer', 'model': 'openai-oauth:gpt-6.1-sol'}),
            ('swarm', {'action': 'spawn', 'label': 'worker', 'model': 'x5-copilot:x5-airun-code-large'}),
            ('swarm', {'action': 'spawn', 'label': 'worker', 'model': 'openai-oauth:gpt-6.1-terra'}),
            ('swarm', {'action': 'status'}),
            ('bash', {'action': 'spawn'}),
        ):
            with self.subTest(tool=tool, payload=payload):
                self.assertEqual(run_hook(tool, payload).returncode, 0)

    def test_dispatch_that_can_spawn_requires_explicit_model(self):
        for action in ('assign_task', 'assign', 'assign_next', 'fill_slots'):
            for flag in ('prefer_spawn', 'spawn_if_needed'):
                with self.subTest(action=action, flag=flag):
                    self.assertEqual(run_hook('swarm', {'action': action, flag: True}).returncode, 2)
                    self.assertEqual(run_hook('swarm', {'action': action, flag: True, 'model': 'openai-oauth:gpt-6.1-terra'}).returncode, 0)
        self.assertEqual(run_hook('swarm', {'action': 'run_plan'}).returncode, 2)

    def test_reuse_only_dispatch_does_not_require_a_model(self):
        for action in ('assign_task', 'assign', 'assign_next', 'fill_slots'):
            with self.subTest(action=action):
                self.assertEqual(run_hook('swarm', {'action': action}).returncode, 0)
                self.assertEqual(run_hook('swarm', {'action': action, 'target_session': 'existing', 'prefer_spawn': True}).returncode, 0)

    def test_malformed_input_does_not_block_unrelated_tools(self):
        result = subprocess.run([sys.executable, str(HOOK)], input='not json', text=True,
                                capture_output=True, env=os.environ | {'JCODE_HOOK_TOOL_NAME': 'bash'}, check=False)
        self.assertEqual(result.returncode, 0)


if __name__ == '__main__':
    unittest.main()
