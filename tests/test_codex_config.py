from __future__ import annotations

import importlib.util
import stat
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "codex_config", ROOT / "scripts/codex_config.py"
)
assert SPEC and SPEC.loader
codex_config = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = codex_config
SPEC.loader.exec_module(codex_config)


class CodexConfigTest(unittest.TestCase):
    def test_sync_masks_secrets_and_excludes_runtime_state(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            home = Path("/Users/alice")
            source = tmp_path / "live.toml"
            target = tmp_path / "snapshot.toml"
            source.write_text(
                """developer_instructions = "generated"
model = "gpt-test"
model_provider = "openai"
personality = "pragmatic"
model_reasoning_effort = "high"

[projects."/Users/alice/work"]
trust_level = "trusted"

[shell_environment_policy]
inherit = "core"

[mcp_servers.zai-mcp-server]
command = "npx"

[mcp_servers.zai-mcp-server.env]
Z_AI_API_KEY = "real-zai-secret"
Z_AI_MODE = "ZAI"

[mcp_servers.aws]
command = "aws-mcp"

[mcp_servers.aws.env]
AWS_ACCESS_KEY_ID = "AKIAFAKEEXAMPLE1234"
SESSION_COOKIE = "synthetic-cookie-value"

[shell_environment_policy.set]
SHELL_SESSION_COOKIE = "synthetic-shell-cookie-value"

[model_providers.cf-openai]
name = "gateway"
base_url = "https://gateway.example/private-account"
requires_openai_auth = true

[tui]
status_line = ["model"]
last_session_id = "synthetic-runtime-session"

[tui.model_availability_nux]
"gpt-test" = 4

[features]
plugin_hooks = true

[hooks.state."/Users/alice/.codex/hooks.json:stop:0:0"]
trusted_hash = "sha256:runtime"
""",
                encoding="utf-8",
            )

            changed, placeholders = codex_config.sync_config(source, target, home=home)

            self.assertTrue(changed)
            self.assertEqual(
                placeholders,
                (
                    "AWS_ACCESS_KEY_ID",
                    "CF_OPENAI_BASE_URL",
                    "SESSION_COOKIE",
                    "SHELL_SESSION_COOKIE",
                    "Z_AI_API_KEY",
                ),
            )
            text = target.read_text(encoding="utf-8")
            parsed = tomllib.loads(text)
            self.assertNotIn("developer_instructions", parsed)
            self.assertNotIn("projects", parsed)
            self.assertNotIn("features", parsed)
            self.assertNotIn("hooks", parsed)
            self.assertNotIn("model_availability_nux", parsed["tui"])
            self.assertNotIn("last_session_id", parsed["tui"])
            self.assertEqual(
                parsed["mcp_servers"]["zai-mcp-server"]["env"]["Z_AI_API_KEY"],
                "${Z_AI_API_KEY}",
            )
            self.assertEqual(
                parsed["model_providers"]["cf-openai"]["base_url"],
                "${CF_OPENAI_BASE_URL}",
            )
            self.assertEqual(
                parsed["mcp_servers"]["aws"]["env"]["AWS_ACCESS_KEY_ID"],
                "${AWS_ACCESS_KEY_ID}",
            )
            self.assertEqual(
                parsed["mcp_servers"]["aws"]["env"]["SESSION_COOKIE"],
                "${SESSION_COOKIE}",
            )
            self.assertEqual(
                parsed["shell_environment_policy"]["set"]["SHELL_SESSION_COOKIE"],
                "${SHELL_SESSION_COOKIE}",
            )
            self.assertNotIn("real-zai-secret", text)
            self.assertNotIn("AKIAFAKEEXAMPLE1234", text)
            self.assertNotIn("synthetic-cookie-value", text)
            self.assertNotIn("synthetic-shell-cookie-value", text)
            self.assertNotIn("synthetic-runtime-session", text)
            self.assertNotIn(str(home), text)

    def test_sync_fails_closed_for_secret_args_and_static_auth_headers(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            cases = {
                "opaque auth argument": """[mcp_servers.unsafe]
command = "unsafe-mcp"
args = ["--auth", "opaque-value"]
""",
                "static header": """[mcp_servers.unsafe]
url = "https://mcp.example.test"

[mcp_servers.unsafe.http_headers]
X-Region = "opaque-static-value"
""",
                "credential query": """[mcp_servers.unsafe]
url = "https://mcp.example.test?access_key=opaque-value"
""",
                "signed query": """[mcp_servers.unsafe]
url = "https://mcp.example.test?X-Amz-Signature=opaque-value"
""",
                "oauth code query": """[mcp_servers.unsafe]
url = "https://mcp.example.test/callback?code=opaque-value"
""",
                "credential path": """[mcp_servers.unsafe]
url = "https://mcp.example.test/mcp/private-token/opaque-value"
""",
                "static header argument": """[mcp_servers.unsafe]
command = "unsafe-mcp"
args = ["--header", "Authorization: opaque-private-credential"]
""",
                "credential endpoint argument": """[mcp_servers.unsafe]
command = "unsafe-mcp"
args = ["--endpoint", "https://alice:opaque-private-credential@example.test/mcp"]
""",
                "secret env argument": """[mcp_servers.unsafe]
command = "unsafe-mcp"
args = ["--env", "API_KEY=opaque-private-credential"]
""",
                "camel token key": """[model_providers.unsafe]
name = "unsafe"
accessToken = "opaque-private-credential"
""",
                "numeric static header": """[mcp_servers.unsafe]
url = "https://mcp.example.test"

[mcp_servers.unsafe.http_headers]
X-Private = 123456789
""",
                "non-string MCP environment": """[mcp_servers.unsafe]
command = "unsafe-mcp"

[mcp_servers.unsafe.env]
PRIVATE_VALUE = 123456789
""",
                "opaque bearer env reference": """[mcp_servers.unsafe]
url = "https://mcp.example.test"
bearer_token_env_var = "opaque-private-credential"
""",
                "numeric provider env reference": """[model_providers.unsafe]
name = "unsafe"
env_key = 123456789
""",
                "opaque header env reference": """[mcp_servers.unsafe]
url = "https://mcp.example.test"

[mcp_servers.unsafe.env_http_headers]
Authorization = "opaque-private-credential"
""",
                "secret auth container": """[mcp_servers.unsafe.auth]
value = "opaque-private-credential"
""",
                "secret list container": """[model_providers.unsafe]
name = "unsafe"
api_keys = ["opaque-private-credential"]
""",
                "secret inline container": """[agents]
credentials = { primary = "opaque-private-credential" }
""",
            }
            for name, mcp_config in cases.items():
                with self.subTest(name=name):
                    source = tmp_path / f"{name}.toml"
                    target = tmp_path / f"{name}.snapshot.toml"
                    source.write_text(
                        f'model = "gpt-test"\n\n{mcp_config}', encoding="utf-8"
                    )
                    with self.assertRaises(ValueError):
                        codex_config.sync_config(
                            source, target, home=Path("/Users/alice")
                        )
                    self.assertFalse(target.exists())

    def test_sync_regenerates_tables_without_copying_comments(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            source = tmp_path / "live.toml"
            target = tmp_path / "snapshot.toml"
            source.write_text(
                """model = "gpt-test"

[mcp_servers.safe]
# PRIVATE_NOTE=opaque-value-that-must-not-be-copied
command = "safe-mcp"
bearer_token_env_var = "SAFE_TOKEN_ENV_NAME"
""",
                encoding="utf-8",
            )

            codex_config.sync_config(source, target, home=Path("/Users/alice"))

            self.assertNotIn("PRIVATE_NOTE", target.read_text(encoding="utf-8"))

    def test_sync_detects_source_change_even_when_snapshot_was_current(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            source = tmp_path / "live.toml"
            target = tmp_path / "snapshot.toml"
            source.write_text('model = "old-model"\n', encoding="utf-8")
            codex_config.sync_config(source, target, home=Path("/Users/alice"))
            snapshot_bytes = target.read_bytes()
            original_leak_gate = codex_config._leak_gate

            def mutate_source(snapshot: str, home: Path) -> None:
                original_leak_gate(snapshot, home)
                source.write_text('model = "concurrent-model"\n', encoding="utf-8")

            with mock.patch.object(
                codex_config, "_leak_gate", side_effect=mutate_source
            ):
                with self.assertRaisesRegex(ValueError, "changed while"):
                    codex_config.sync_config(source, target, home=Path("/Users/alice"))

            self.assertEqual(target.read_bytes(), snapshot_bytes)
            self.assertIn("concurrent-model", source.read_text(encoding="utf-8"))

    def test_apply_merges_stable_settings_and_preserves_runtime_tables(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text(
                """model = "new-model"
model_reasoning_effort = "high"

[mcp_servers.browser]
command = "new-browser"

[mcp_servers.zai-mcp-server]
command = "zai"

[mcp_servers.zai-mcp-server.env]
Z_AI_API_KEY = "${Z_AI_API_KEY}"

[model_providers.cf-openai]
base_url = "${CF_OPENAI_BASE_URL}"
requires_openai_auth = true

[tui]
status_line = ["model"]
""",
                encoding="utf-8",
            )
            target.write_text(
                """developer_instructions = "generated"
model = "old-model"

[projects."/work"]
trust_level = "trusted"

[mcp_servers.old]
command = "remove-me"

[features]
plugin_hooks = true

[tui]
last_session_id = "runtime-session"

[tui.model_availability_nux]
"new-model" = 4

[hooks.state."source"]
trusted_hash = "sha256:runtime"

[plugins."omx"]
enabled = true
""",
                encoding="utf-8",
            )

            result = codex_config.apply_config(
                template,
                target,
                values={
                    "Z_AI_API_KEY": "rendered-zai-secret",
                    "CF_OPENAI_BASE_URL": "https://gateway.example/rendered",
                },
            )

            self.assertTrue(result.changed)
            self.assertIsNotNone(result.backup)
            assert result.backup is not None
            self.assertEqual(stat.S_IMODE(result.backup.stat().st_mode), 0o600)
            self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o600)
            parsed = tomllib.loads(target.read_text(encoding="utf-8"))
            self.assertEqual(parsed["model"], "new-model")
            self.assertEqual(parsed["developer_instructions"], "generated")
            self.assertIn("projects", parsed)
            self.assertIn("features", parsed)
            self.assertIn("hooks", parsed)
            self.assertIn("plugins", parsed)
            self.assertEqual(parsed["tui"]["last_session_id"], "runtime-session")
            self.assertIn("model_availability_nux", parsed["tui"])
            self.assertNotIn("old", parsed["mcp_servers"])
            self.assertEqual(
                parsed["mcp_servers"]["zai-mcp-server"]["env"]["Z_AI_API_KEY"],
                "rendered-zai-secret",
            )

            second = codex_config.apply_config(
                template,
                target,
                values={
                    "Z_AI_API_KEY": "rendered-zai-secret",
                    "CF_OPENAI_BASE_URL": "https://gateway.example/rendered",
                },
            )
            self.assertFalse(second.changed)
            self.assertIsNone(second.backup)

    def test_apply_rejects_runtime_owned_template_keys_semantically(self) -> None:
        cases = {
            "quoted generated instructions": '"developer_instructions" = "owned"\n',
            "inline project trust": (
                '"projects" = { "/tmp" = { trust_level = "trusted" } }\n'
            ),
            "runtime TUI sibling": (
                '[tui]\nstatus_line = ["model"]\nlast_session_id = "owned"\n'
            ),
        }
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            for index, (name, template_text) in enumerate(cases.items()):
                with self.subTest(name=name):
                    template = tmp_path / f"template-{index}.toml"
                    target = tmp_path / f"target-{index}.toml"
                    template.write_text(template_text, encoding="utf-8")
                    target.write_text('model = "local-model"\n', encoding="utf-8")
                    original = target.read_bytes()

                    with self.assertRaisesRegex(ValueError, "unmanaged settings"):
                        codex_config.apply_config(template, target, values={})

                    self.assertEqual(target.read_bytes(), original)
                    self.assertEqual(
                        list(tmp_path.glob(f"target-{index}.toml.bak-*")), []
                    )

    def test_apply_replaces_quoted_live_root_key(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text('model = "new-model"\n', encoding="utf-8")
            target.write_text(
                '"model" = "old-model"\ndeveloper_instructions = "keep"\n',
                encoding="utf-8",
            )

            result = codex_config.apply_config(template, target, values={})

            self.assertTrue(result.changed)
            parsed = tomllib.loads(target.read_text(encoding="utf-8"))
            self.assertEqual(parsed["model"], "new-model")
            self.assertEqual(parsed["developer_instructions"], "keep")

    def test_apply_replaces_quoted_live_partial_key(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text(
                '[tui]\nstatus_line = ["new-status"]\n', encoding="utf-8"
            )
            target.write_text(
                '[tui]\n"status_line" = ["old-status"]\nlocal_note = "keep"\n',
                encoding="utf-8",
            )

            result = codex_config.apply_config(template, target, values={})

            self.assertTrue(result.changed)
            parsed = tomllib.loads(target.read_text(encoding="utf-8"))
            self.assertEqual(parsed["tui"]["status_line"], ["new-status"])
            self.assertEqual(parsed["tui"]["local_note"], "keep")

    def test_apply_preserves_table_like_lines_inside_multiline_root_string(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text('model = "new-model"\n', encoding="utf-8")
            live_text = '''model = "old-model"
developer_instructions = """
These are literal examples, not TOML tables:
[tui]
[projects.fake]
Keep every line in this instruction.
"""

[features]
plugin_hooks = true
'''
            target.write_text(live_text, encoding="utf-8")
            expected_instructions = tomllib.loads(live_text)["developer_instructions"]

            result = codex_config.apply_config(template, target, values={})

            self.assertTrue(result.changed)
            parsed = tomllib.loads(target.read_text(encoding="utf-8"))
            self.assertEqual(parsed["model"], "new-model")
            self.assertEqual(parsed["developer_instructions"], expected_instructions)
            self.assertNotIn("projects", parsed)
            self.assertNotIn("tui", parsed)
            self.assertTrue(parsed["features"]["plugin_hooks"])

    def test_apply_ignores_managed_lines_inside_unmanaged_multiline_values(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text(
                """model = "new-model"

[tui]
status_line = ["new-status"]
""",
                encoding="utf-8",
            )
            live_text = '''model = "old-model"
developer_instructions = """
The following lines are literal documentation:
model = "text"
# >>> dots Codex managed root >>>
This text is not a managed root block.
# <<< dots Codex managed root <<<
"""

[tui]
local_note = """
The following assignment is literal documentation:
status_line = ["not", "managed"]
"""
status_line = ["old-status"]

[features]
plugin_hooks = true
'''
            target.write_text(live_text, encoding="utf-8")
            parsed_live = tomllib.loads(live_text)
            expected_instructions = parsed_live["developer_instructions"]
            expected_local_note = parsed_live["tui"]["local_note"]

            result = codex_config.apply_config(template, target, values={})

            self.assertTrue(result.changed)
            parsed = tomllib.loads(target.read_text(encoding="utf-8"))
            self.assertEqual(parsed["model"], "new-model")
            self.assertEqual(parsed["developer_instructions"], expected_instructions)
            self.assertEqual(parsed["tui"]["local_note"], expected_local_note)
            self.assertEqual(parsed["tui"]["status_line"], ["new-status"])
            self.assertTrue(parsed["features"]["plugin_hooks"])

            removed = codex_config.remove_config(template, target)
            self.assertTrue(removed.changed)
            parsed_after_remove = tomllib.loads(target.read_text(encoding="utf-8"))
            self.assertNotIn("model", parsed_after_remove)
            self.assertEqual(
                parsed_after_remove["developer_instructions"], expected_instructions
            )
            self.assertEqual(
                parsed_after_remove["tui"]["local_note"], expected_local_note
            )
            self.assertNotIn("status_line", parsed_after_remove["tui"])
            self.assertTrue(parsed_after_remove["features"]["plugin_hooks"])

    def test_apply_validator_failure_leaves_target_and_mode_unchanged(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text('model = "new-model"\n', encoding="utf-8")
            target.write_text('model = "old-model"\n', encoding="utf-8")
            target.chmod(0o640)
            original_bytes = target.read_bytes()
            original_mode = stat.S_IMODE(target.stat().st_mode)

            def reject_config(_text: str) -> None:
                raise ValueError("synthetic validator failure")

            with self.assertRaisesRegex(ValueError, "synthetic validator failure"):
                codex_config.apply_config(
                    template,
                    target,
                    values={},
                    validator=reject_config,
                )

            self.assertEqual(target.read_bytes(), original_bytes)
            self.assertEqual(stat.S_IMODE(target.stat().st_mode), original_mode)
            self.assertEqual(list(tmp_path.glob("config.toml.bak-*")), [])

    def test_apply_rejects_template_target_alias_without_modification(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            config = Path(tmp) / "config.toml"
            config.write_text('model = "existing-model"\n', encoding="utf-8")
            config.chmod(0o640)
            original_bytes = config.read_bytes()
            original_mode = stat.S_IMODE(config.stat().st_mode)

            with self.assertRaises(ValueError):
                codex_config.apply_config(config, config, values={})

            self.assertEqual(config.read_bytes(), original_bytes)
            self.assertEqual(stat.S_IMODE(config.stat().st_mode), original_mode)
            self.assertEqual(list(config.parent.glob("config.toml.bak-*")), [])

    def test_apply_detects_validator_concurrent_mutation_without_overwrite(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text('model = "new-model"\n', encoding="utf-8")
            target.write_text('model = "old-model"\n', encoding="utf-8")
            concurrent_bytes = b'model = "concurrent-model"\n'

            def mutate_target(_text: str) -> None:
                target.write_bytes(concurrent_bytes)

            with self.assertRaisesRegex(ValueError, "changed while"):
                codex_config.apply_config(
                    template,
                    target,
                    values={},
                    validator=mutate_target,
                )

            self.assertEqual(target.read_bytes(), concurrent_bytes)
            self.assertEqual(list(tmp_path.glob("config.toml.bak-*")), [])

    def test_missing_secrets_preserve_existing_groups_but_skip_them_when_fresh(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            existing = tmp_path / "existing.toml"
            fresh = tmp_path / "fresh.toml"
            template.write_text(
                """model = "new-model"

[mcp_servers.zai-mcp-server]
command = "zai"

[mcp_servers.zai-mcp-server.env]
Z_AI_API_KEY = "${Z_AI_API_KEY}"
""",
                encoding="utf-8",
            )
            existing.write_text(
                """model = "old-model"

[mcp_servers.zai-mcp-server]
command = "zai"

[mcp_servers.zai-mcp-server.env]
Z_AI_API_KEY = "keep-existing-secret"
""",
                encoding="utf-8",
            )

            result = codex_config.apply_config(template, existing, values={})
            parsed_existing = tomllib.loads(existing.read_text(encoding="utf-8"))
            self.assertEqual(result.missing_groups, ("mcp_servers.zai-mcp-server",))
            self.assertEqual(result.preserved_missing_groups, result.missing_groups)
            self.assertEqual(
                parsed_existing["mcp_servers"]["zai-mcp-server"]["env"]["Z_AI_API_KEY"],
                "keep-existing-secret",
            )
            removed = codex_config.remove_config(template, existing)
            self.assertTrue(removed.changed)  # the managed root marker is removed
            parsed_after_remove = tomllib.loads(existing.read_text(encoding="utf-8"))
            self.assertEqual(
                parsed_after_remove["mcp_servers"]["zai-mcp-server"]["env"][
                    "Z_AI_API_KEY"
                ],
                "keep-existing-secret",
            )

            fresh_result = codex_config.apply_config(template, fresh, values={})
            parsed_fresh = tomllib.loads(fresh.read_text(encoding="utf-8"))
            self.assertEqual(fresh_result.missing_groups, result.missing_groups)
            self.assertNotIn("mcp_servers", parsed_fresh)

    def test_missing_secret_group_keeps_template_relative_order(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text(
                """[mcp_servers.browser]
command = "browser"

[mcp_servers.private]
command = "private"

[mcp_servers.private.env]
API_KEY = "${API_KEY}"

[mcp_servers.mobile]
command = "mobile"
""",
                encoding="utf-8",
            )
            target.write_text(
                """[mcp_servers.browser]
command = "old-browser"

[mcp_servers.private]
command = "private"

[mcp_servers.private.env]
API_KEY = "keep-private"

[mcp_servers.mobile]
command = "old-mobile"
""",
                encoding="utf-8",
            )

            result = codex_config.apply_config(template, target, values={})

            self.assertEqual(result.preserved_missing_groups, ("mcp_servers.private",))
            applied = target.read_text(encoding="utf-8")
            self.assertLess(
                applied.index("[mcp_servers.browser]"),
                applied.index("[mcp_servers.private]"),
            )
            self.assertLess(
                applied.index("[mcp_servers.private.env]"),
                applied.index("[mcp_servers.mobile]"),
            )

    def test_remove_deletes_only_template_owned_settings(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text(
                """model = "managed-model"

[mcp_servers.managed]
command = "managed-mcp"

[tui]
status_line = ["model"]
""",
                encoding="utf-8",
            )
            target.write_text(
                """# >>> dots Codex managed root >>>
model = "managed-model"
# <<< dots Codex managed root <<<

developer_instructions = "generated"

[projects."/work"]
trust_level = "trusted"

[mcp_servers.managed]
# >>> dots Codex managed table >>>
command = "managed-mcp"
# <<< dots Codex managed table <<<

[mcp_servers.local-only]
command = "keep-me"

[tui]
# >>> dots Codex managed keys >>>
status_line = [
  "model",
  "git-branch",
]
# <<< dots Codex managed keys <<<
last_session_id = "keep-runtime"

[tui.model_availability_nux]
"managed-model" = 4

[plugins."omx"]
enabled = true
""",
                encoding="utf-8",
            )

            result = codex_config.remove_config(template, target)

            self.assertTrue(result.changed)
            parsed = tomllib.loads(target.read_text(encoding="utf-8"))
            self.assertNotIn("model", parsed)
            self.assertEqual(parsed["developer_instructions"], "generated")
            self.assertIn("projects", parsed)
            self.assertNotIn("managed", parsed["mcp_servers"])
            self.assertEqual(parsed["mcp_servers"]["local-only"]["command"], "keep-me")
            self.assertNotIn("status_line", parsed["tui"])
            self.assertEqual(parsed["tui"]["last_session_id"], "keep-runtime")
            self.assertIn("model_availability_nux", parsed["tui"])
            self.assertIn("plugins", parsed)
            self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o600)

    def test_remove_is_byte_and_mode_noop_without_complete_ownership_markers(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            template = tmp_path / "template.toml"
            target = tmp_path / "config.toml"
            template.write_text(
                'model = "managed-model"\n\n[mcp_servers.managed]\ncommand = "mcp"\n',
                encoding="utf-8",
            )
            target.write_text(
                """model = "local-model"

[mcp_servers.local]
# >>> dots Codex managed table >>>
command = "keep-with-unpaired-marker"
""",
                encoding="utf-8",
            )
            target.chmod(0o640)
            original_bytes = target.read_bytes()
            original_mode = stat.S_IMODE(target.stat().st_mode)

            result = codex_config.remove_config(template, target)

            self.assertFalse(result.changed)
            self.assertEqual(result.removed_groups, ())
            self.assertEqual(target.read_bytes(), original_bytes)
            self.assertEqual(stat.S_IMODE(target.stat().st_mode), original_mode)


if __name__ == "__main__":
    unittest.main()
