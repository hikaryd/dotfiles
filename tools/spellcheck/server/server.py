import argparse
import difflib
import os
import re
import signal
import sys
import threading
import time
from contextlib import asynccontextmanager
from typing import Optional

import torch
import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from transformers import AutoModelForSeq2SeqLM, AutoTokenizer

DEFAULT_MODEL = os.environ.get("SPELLCHECK_MODEL", "ai-forever/sage-fredt5-large")
MAX_INPUT_CHARS = 400
URL_OR_CODE_RE = re.compile(r"(https?://|www\.|\b[A-Za-z0-9_]+\.[a-z]{2,4}/|=>|::|[{}<>])")
# Matches Latin identifiers/terms (e.g. Jev, Mac, Docker, gpt-4, v1.2) without capturing trailing `.` or `-`.
LATIN_TOKEN_RE = re.compile(r"(?<![A-Za-z0-9_])[A-Za-z][A-Za-z0-9_]*(?:[.-][A-Za-z0-9_]+)*(?![A-Za-z0-9_])")
TRAILING_PUNCT_RE = re.compile(r"[.!?]+$")


def should_skip_input(text: str) -> bool:
    s = text.strip()
    if len(s) < 6 or len(s) > MAX_INPUT_CHARS:
        return True
    if URL_OR_CODE_RE.search(s):
        return True
    words = re.findall(r"[А-Яа-яЁё]+", s)
    return len(words) < 2


def _normalize_trivial(s: str) -> str:
    """Normalizes casing of first char, trailing period, and е/ё to detect trivial-only diffs."""
    t = s.strip()
    if t.endswith(".") and not t.endswith("..."):
        t = t[:-1].rstrip()
    t = t.replace("ё", "е").replace("Ё", "Е")
    if t:
        t = t[0].lower() + t[1:]
    return t


def _restore_latin_tokens(original: str, corrected: str) -> str:
    """Restores Latin tokens mutated by the Russian model (e.g. `Jev` -> `Javel`),
    matching each mutated token to an unmatched original Latin token only when clearly cognate."""
    orig_tokens = LATIN_TOKEN_RE.findall(original)
    if not orig_tokens:
        return corrected
    corr_matches = list(LATIN_TOKEN_RE.finditer(corrected))
    if not corr_matches:
        return corrected

    # Track counts of exact matches so intact tokens are never stolen by another slot.
    unmatched_orig = list(orig_tokens)
    exact_corr_indices: set[int] = set()
    for idx, m in enumerate(corr_matches):
        tok = m.group(0)
        if tok in unmatched_orig:
            unmatched_orig.remove(tok)
            exact_corr_indices.add(idx)

    if not unmatched_orig:
        return corrected

    replacements: dict[int, str] = {}
    for idx, m in enumerate(corr_matches):
        if idx in exact_corr_indices:
            continue
        c_tok = m.group(0)
        c_low = c_tok.lower()
        best_pos = -1
        best_score = 0.0
        for o_idx, o_tok in enumerate(unmatched_orig):
            o_low = o_tok.lower()
            if o_low[0] != c_low[0]:
                continue
            score = difflib.SequenceMatcher(None, o_low, c_low).ratio()
            if score >= 0.45 and score > best_score:
                best_score = score
                best_pos = o_idx
        if best_pos >= 0:
            replacements[idx] = unmatched_orig.pop(best_pos)

    if not replacements:
        return corrected

    parts = []
    last_end = 0
    for idx, m in enumerate(corr_matches):
        if idx in replacements:
            parts.append(corrected[last_end : m.start()])
            parts.append(replacements[idx])
            last_end = m.end()
    parts.append(corrected[last_end:])
    return "".join(parts)


def postprocess_correction(original: str, raw_corrected: str) -> tuple[str, bool]:
    """Applies hallucination, Latin-preservation, punctuation, and casual-chat noise guards."""
    orig = original.strip()
    corr = re.sub(r"\s+", " ", raw_corrected).strip()

    if not corr:
        return orig, False

    # Hallucination / truncation guard: reject wild length divergence.
    ratio = len(corr) / max(1, len(orig))
    if ratio < 0.65 or ratio > 1.45:
        return orig, False

    # Never let the Russian model mutate Latin tech terms or product names.
    corr = _restore_latin_tokens(orig, corr)

    # Restore dropped trailing punctuation cluster (`.`, `!`, `?`, `?!`, `...`).
    orig_tail_m = TRAILING_PUNCT_RE.search(orig)
    corr_tail_m = TRAILING_PUNCT_RE.search(corr)
    if orig_tail_m:
        orig_tail = orig_tail_m.group(0)
        if not corr_tail_m:
            corr = corr + orig_tail
        elif len(orig_tail) > 1 and corr_tail_m.group(0) != orig_tail and corr_tail_m.group(0) in orig_tail:
            corr = corr[: corr_tail_m.start()] + orig_tail
    elif corr.endswith(".") and not corr.endswith("..."):
        # In casual chats without trailing punctuation, don't force a trailing period.
        corr = corr[:-1].rstrip()

    # Preserve lowercase initial letter when user intentionally typed lowercase (casual chat).
    if orig[0].islower() and corr and corr[0].isupper():
        if len(corr) == 1 or not corr[1].isupper():
            corr = corr[0].lower() + corr[1:]

    # Suppress trivial-only diffs (only е/ё, leading capitalization, or trailing period).
    if _normalize_trivial(orig) == _normalize_trivial(corr):
        return orig, False

    if corr == orig:
        return orig, False

    return corr, True


class ModelRuntime:
    def __init__(self, model_name: str):
        self.model_name = model_name
        self.device = "mps" if torch.backends.mps.is_available() else "cpu"
        self.dtype = torch.bfloat16 if self.device == "mps" else torch.float32
        self.tokenizer = None
        self.model = None
        self.status = "loading"
        self.error: Optional[str] = None
        self._lock = threading.Lock()

    def load(self) -> None:
        try:
            tok = AutoTokenizer.from_pretrained(self.model_name)
            mdl = AutoModelForSeq2SeqLM.from_pretrained(
                self.model_name, dtype=self.dtype
            ).to(self.device)
            mdl.eval()
            if getattr(mdl, "generation_config", None) is not None:
                mdl.generation_config.max_length = None

            # Warmup pass so Metal/MPS kernels compile before first real keystroke.
            inp = tok("Привет мир.", return_tensors="pt").to(self.device)
            with torch.inference_mode():
                _ = mdl.generate(**inp, max_new_tokens=32, num_beams=1, do_sample=False)
            if self.device == "mps":
                torch.mps.synchronize()

            self.tokenizer = tok
            self.model = mdl
            self.status = "ok"
        except Exception as exc:
            self.error = str(exc)
            self.status = "error"

    def correct(self, text: str) -> tuple[str, bool, float]:
        if self.status != "ok" or self.model is None or self.tokenizer is None:
            raise RuntimeError(f"Model not ready (status={self.status})")

        t0 = time.perf_counter()
        if should_skip_input(text):
            return text.strip(), False, (time.perf_counter() - t0) * 1000.0

        with self._lock:
            inp = self.tokenizer(
                text.strip(),
                max_length=256,
                truncation=True,
                return_tensors="pt",
            ).to(self.device)
            max_new = min(256, int(inp["input_ids"].shape[1] * 1.45) + 16)
            with torch.inference_mode():
                out = self.model.generate(
                    **inp,
                    max_new_tokens=max_new,
                    num_beams=1,
                    do_sample=False,
                )
            if self.device == "mps":
                torch.mps.synchronize()
            raw = self.tokenizer.decode(
                out[0], skip_special_tokens=True, clean_up_tokenization_spaces=False
            )

        corrected, changed = postprocess_correction(text, raw)
        latency_ms = (time.perf_counter() - t0) * 1000.0
        return corrected, changed, latency_ms


class CheckRequest(BaseModel):
    text: str = Field(..., max_length=600)


class CheckResponse(BaseModel):
    original: str
    corrected: str
    changed: bool
    latency_ms: float


def _start_parent_watchdog(parent_pid: Optional[int], watch_stdin: bool) -> None:
    """Terminates the server cleanly if the parent app dies (even on SIGKILL/crash)."""
    if watch_stdin:
        def _stdin_watcher() -> None:
            try:
                while True:
                    chunk = sys.stdin.buffer.read(1)
                    if not chunk:
                        break
            except Exception:
                pass
            os.kill(os.getpid(), signal.SIGTERM)

        threading.Thread(target=_stdin_watcher, daemon=True, name="stdin-watchdog").start()

    if parent_pid is not None and parent_pid > 1:
        def _ppid_watcher() -> None:
            while True:
                time.sleep(1.0)
                if os.getppid() != parent_pid:
                    os.kill(os.getpid(), signal.SIGTERM)
                    return

        threading.Thread(target=_ppid_watcher, daemon=True, name="ppid-watchdog").start()


def create_app(model_name: str = DEFAULT_MODEL, preload: bool = True) -> FastAPI:
    runtime = ModelRuntime(model_name)

    @asynccontextmanager
    async def lifespan(_: FastAPI):
        if preload:
            t = threading.Thread(target=runtime.load, daemon=True)
            t.start()
        yield

    app = FastAPI(title="SpellcheckMac Local Server", lifespan=lifespan)
    app.state.runtime = runtime

    @app.get("/health")
    def health():
        return {
            "status": runtime.status,
            "model": runtime.model_name.split("/")[-1],
            "device": runtime.device,
            "error": runtime.error,
        }

    @app.post("/check", response_model=CheckResponse)
    def check(req: CheckRequest):
        if runtime.status != "ok":
            raise HTTPException(status_code=503, detail=f"Model {runtime.status}")
        corrected, changed, latency_ms = runtime.correct(req.text)
        return CheckResponse(
            original=req.text.strip(),
            corrected=corrected,
            changed=changed,
            latency_ms=round(latency_ms, 1),
        )

    return app


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18765)
    parser.add_argument("--model", default=DEFAULT_MODEL)
    parser.add_argument("--parent-pid", type=int, default=None)
    parser.add_argument("--watch-stdin", action="store_true")
    args = parser.parse_args()

    # Enforce loopback binding so private typed text is never exposed on LAN.
    if args.host not in ("127.0.0.1", "localhost", "::1"):
        raise SystemExit("Refusing to bind to non-loopback address")

    _start_parent_watchdog(args.parent_pid, args.watch_stdin)
    app = create_app(model_name=args.model, preload=True)
    uvicorn.run(app, host=args.host, port=args.port, log_level="warning")


if __name__ == "__main__":
    main()
