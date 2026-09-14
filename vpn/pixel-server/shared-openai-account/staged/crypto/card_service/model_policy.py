"""Service-local model and cost policy. No global Hermes configuration."""
from __future__ import annotations

from dataclasses import dataclass

PROVIDER = "openai-codex"
IMPLEMENTATION_MODEL = "gpt-5.6-sol"
RESEARCH_MODELS = ("gpt-5.6-terra", "gpt-5.6-luna")
ALLOWED_MODELS = (*RESEARCH_MODELS, IMPLEMENTATION_MODEL)
DEFAULT_RESEARCH_MODEL = "gpt-5.6-terra"

# Hard service ceilings. A caller may lower but never raise these.
MAX_STAGE_INPUT_TOKENS = 18_000
MAX_STAGE_OUTPUT_TOKENS = 4_000
MAX_STAGE_CALLS = 2  # initial + one schema repair
MAX_RUN_CALLS = 160
MAX_RUN_INPUT_TOKENS = 1_500_000
MAX_RUN_OUTPUT_TOKENS = 240_000

RESEARCH_STAGES = frozenset({
    "extraction", "summarization", "source_classification", "review_processing",
})
IMPLEMENTATION_STAGES = frozenset({"implementation", "coding"})

@dataclass(frozen=True)
class Selection:
    stage: str
    model: str
    provider: str = PROVIDER


def select(stage: str, requested_model: str | None = None) -> Selection:
    if stage in RESEARCH_STAGES:
        model = requested_model or DEFAULT_RESEARCH_MODEL
        if model not in RESEARCH_MODELS:
            raise ValueError("research stage requires an explicitly allowed lightweight model")
    elif stage in IMPLEMENTATION_STAGES:
        model = requested_model or IMPLEMENTATION_MODEL
        if model != IMPLEMENTATION_MODEL:
            raise ValueError("implementation stage requires gpt-5.6-sol")
    else:
        raise ValueError("unknown model stage")
    return Selection(stage=stage, model=model)
