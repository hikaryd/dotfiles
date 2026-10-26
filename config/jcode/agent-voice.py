import contextlib
import fcntl
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unicodedata

CACHE = Path(os.environ.get("AGENT_VOICE_CACHE_DIR", Path.home() / ".cache/agent-voice"))
MODEL = "mlx-community/Qwen3-TTS-12Hz-1.7B-VoiceDesign-4bit"
VOICE = (
    "A young adult woman with a bright, cute anime-style voice, high pitch, "
    "warm and playful but clear and not shrill. Natural fluent Russian pronunciation."
)
_model = None


def _render(text, path):
    global _model
    with contextlib.redirect_stdout(sys.stderr):
        import mlx.core as mx
        import numpy as np
        import soundfile as sf
        from mlx_audio.tts.utils import load_model

        if _model is None:
            _model = load_model(MODEL)
        mx.random.seed(0)
        results = list(_model.generate(
            text=text, instruct=VOICE, lang_code="russian", max_tokens=512,
        ))
        if not results:
            raise RuntimeError("The voice model produced no audio")
        sf.write(path, np.concatenate([np.asarray(r.audio) for r in results]),
                 results[0].sample_rate, subtype="PCM_16")


def speak(text: str) -> dict:
    """Call Egor for input or announce significant completion aloud in Russian.

    Compose only this tool's text as a warm, playful adult anime heroine,
    addressing Егор or Егор-кун, with lively character, not just a cute timbre.
    Keep ordinary chat in its usual style. Use 1-2 short sentences, at most
    300 characters, with no secrets. Already plays audio on this Mac.
    If busy or unavailable, continue in text without retries. Cold model
    loading can take several minutes, so do not launch a duplicate call.
    """
    text = unicodedata.normalize("NFC", text).strip()
    if not text or len(text) > 300:
        raise ValueError("Use 1 to 300 characters for a short spoken notification")
    text = re.sub(r"\bпозову\b(?!\u0301)", lambda m: m[0] + "\u0301", text, flags=re.I)
    CACHE.mkdir(parents=True, exist_ok=True, mode=0o700)
    CACHE.chmod(0o700)
    with (CACHE / "speech.lock").open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return {"played": False, "reason": "busy"}
        fd, path = tempfile.mkstemp(suffix=".wav", dir=CACHE)
        os.close(fd)
        try:
            _render(text, path)
            subprocess.run(["/usr/bin/afplay", path], check=True, timeout=60, capture_output=True)
            return {"played": True}
        finally:
            Path(path).unlink(missing_ok=True)


if __name__ == "__main__":
    os.environ["HF_HUB_OFFLINE"] = "1"
    os.environ["TRANSFORMERS_OFFLINE"] = "1"
    from mcp.server.fastmcp import FastMCP

    server = FastMCP("agent-voice")
    server.add_tool(speak)
    server.run(transport="stdio")
