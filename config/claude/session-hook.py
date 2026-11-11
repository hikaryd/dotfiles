import json
import os
from pathlib import Path

root = Path(__file__).resolve().parent
context = (root / "session-context.txt").read_text()
ponytail = Path(os.environ["CLAUDE_CONFIG_DIR"]) / "skills/ponytail/SKILL.md"
if ponytail.is_file():
    context += "\n\n" + ponytail.read_text()
print(json.dumps({"hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": context}}))
