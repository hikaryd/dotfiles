"""Production composition root for weekly, manual and intake launch paths.

Constructors are explicit and dependency-injectable. ``preflight`` instantiates the
same concrete classes with a local placeholder bearer that is never used; it cannot
perform Google requests, browser capture, model inference, publication or delivery.
Runtime actions remain approval-gated by their systemd units.
"""
from __future__ import annotations

import argparse
import copy
from dataclasses import asdict, dataclass
import json
import os
from pathlib import Path
from typing import Any, Callable, Mapping

from .dynamic_google_transport import DynamicGoogleSheetsTransport
from .dynamic_publication import (
    DynamicPublicationAdapter,
    DynamicPublicationConfig,
    EffectiveDatasetPaths,
    configured_effective_loader,
)
from .intake_consumer import ExistingDetailedResearch, IntakeConsumer
from .intake_queue import IntakeQueue
from .intake_sheets import IntakeSheets
from .live_refresh import research_composition
from .publisher import GoogleSheetsTransport
from .weekly_publication import SHEET_ID


OWNED_RANGES = {
    "Сравнение": "'Сравнение'!A1:P200",
    "Подробности": "'Подробности'!A1:G1000",
    "Тарифы и уровни": "'Тарифы и уровни'!A1:K500",
}
PRODUCTION_ROOT = Path("/opt/crypto-card-service")
COMPOSITION_KEYS = (
    "model_stage", "model", "provider", "account_stage", "account_policy",
    "expected_account_id",
)


@dataclass(frozen=True)
class ServicePaths:
    root: Path

    def __post_init__(self) -> None:
        root = Path(self.root)
        if not root.is_absolute():
            raise ValueError("service root must be absolute")
        object.__setattr__(self, "root", root.resolve())

    @staticmethod
    def source_root() -> Path:
        return Path(__file__).resolve().parents[1]

    @property
    def effective_artifacts(self) -> tuple[str, ...]:
        return (
            "state/snapshots/production-live-20260912-v2.json",
            "output/presentation.ru.json",
            "output/reviewed-coca-delta.json",
            "output/bybit-reviewed-delta.json",
            "output/reviewed-tier-summary-deltas.json",
            "output/priority-tiers.json",
        )

    @property
    def effective(self) -> EffectiveDatasetPaths:
        artifacts = self.effective_artifacts
        return EffectiveDatasetPaths(
            root=self.root,
            snapshot=artifacts[0],
            presentation=artifacts[1],
            deltas=artifacts[2:5],
            tiers=artifacts[5],
        )

    @property
    def intake_db(self) -> Path:
        return self.root / "state/fintech-intake/intake-queue.sqlite"

    @property
    def dynamic_publication_db(self) -> Path:
        return self.root / "state/fintech-intake/dynamic-publication.sqlite"

    @property
    def publisher_root(self) -> Path:
        # WeeklyPublisher(root/state) uses this exact Store root and lock inode.
        return self.root / "state"


def production_composition() -> dict[str, Any]:
    """Resolve production with the owner-selected shared Hermes account policy."""
    result = research_composition(account_stage="production")
    if tuple(result) != COMPOSITION_KEYS:
        raise ValueError("unexpected shared research composition shape")
    return copy.deepcopy(result)


def broker_config(*, allowed_uid: int = 0) -> dict[str, Any]:
    """Map the production composition to the existing Hermes broker schema."""
    if type(allowed_uid) is not int or allowed_uid < 0:
        raise ValueError("explicit broker uid required")
    composition = production_composition()
    return {
        "socket": "/run/crypto-card-subscription/subscription.sock",
        "allowed_uid": allowed_uid,
        "model": composition["model"],
        "provider": composition["provider"],
        "tools": [],
        "paid_fallback": False,
        "stage": composition["account_stage"],
        "account_policy": composition["account_policy"],
        "expected_account_id": composition["expected_account_id"],
    }


def validate_release_config(config: Mapping[str, Any], *, require_enabled: bool | None = None) -> dict[str, Any]:
    if not isinstance(config, Mapping):
        raise ValueError("weekly release config must be an object")
    expected = production_composition()
    for key, value in expected.items():
        if config.get(key) != value:
            raise ValueError("weekly release config composition mismatch")
    enabled = config.get("enabled")
    publication_enabled = config.get("publication_enabled")
    if type(enabled) is not bool or publication_enabled is not enabled:
        raise ValueError("weekly enabled/publication flags must match")
    if require_enabled is not None and enabled is not require_enabled:
        raise ValueError("weekly release activation state mismatch")
    if config.get("paid_fallback") is not False:
        raise ValueError("paid fallback must remain disabled")
    if config.get("scopes") != "config/weekly-scopes.json":
        raise ValueError("unexpected weekly scopes path")
    if config.get("schedule") != "Mon 09:00 Europe/Moscow":
        raise ValueError("unexpected weekly schedule")
    if config.get("delivery_target") != "telegram:464089905":
        raise ValueError("unexpected delivery target")
    budget = config.get("budget_seconds")
    if isinstance(budget, bool) or not isinstance(budget, (int, float)) or not 0 < budget <= 3300:
        raise ValueError("unsafe weekly budget")
    return copy.deepcopy(dict(config))


@dataclass
class ServiceComposition:
    paths: ServicePaths
    composition: Mapping[str, Any]
    queue: IntakeQueue
    research: ExistingDetailedResearch
    publisher: DynamicPublicationAdapter
    consumer: IntakeConsumer
    intake_sheets: IntakeSheets

    def close(self) -> None:
        self.queue.close()
        self.publisher.store.db.close()


def build_service_composition(
    root: str | Path = PRODUCTION_ROOT,
    *,
    sheet_id: str = SHEET_ID,
    intake_transport: Any | None = None,
    dynamic_transport: Any | None = None,
    research_runner: Callable[..., Mapping[str, Any]] | None = None,
    effective_loader: Callable[..., Mapping[str, Any]] = configured_effective_loader,
) -> ServiceComposition:
    paths = ServicePaths(Path(root))
    composition = production_composition()
    # Validate all immutable effective paths before opening a queue or auth adapter.
    paths.effective.validate()
    intake_transport = intake_transport or GoogleSheetsTransport()
    dynamic_transport = dynamic_transport or DynamicGoogleSheetsTransport(OWNED_RANGES)
    queue = IntakeQueue(paths.intake_db)
    try:
        research = ExistingDetailedResearch(
            paths.root / "state",
            composition=composition,
            wall_seconds=3300,
            runner=research_runner,
        )
        publisher = DynamicPublicationAdapter(
            DynamicPublicationConfig(
                paths=paths.effective,
                state_db=paths.dynamic_publication_db,
                publisher_root=paths.publisher_root,
                spreadsheet_id=sheet_id,
                owned_ranges=OWNED_RANGES,
            ),
            transport=dynamic_transport,
            effective_loader=effective_loader,
        )
        consumer = IntakeConsumer(
            queue,
            research,
            publisher,
            lease_seconds=120,
            heartbeat_seconds=30,
            max_work_seconds=3300,
        )
        intake_sheets = IntakeSheets(paths.root, sheet_id, queue, intake_transport)
        return ServiceComposition(paths, composition, queue, research, publisher, consumer, intake_sheets)
    except BaseException:
        queue.close()
        raise


def run_intake_once(composition: ServiceComposition) -> dict[str, Any]:
    """Poll current intake, consume one durable task, then publish current status."""
    before = composition.intake_sheets.poll()
    consumed = composition.consumer.consume_one()
    after = composition.intake_sheets.poll()
    return {
        "poll_before": asdict(before),
        "consume": asdict(consumed),
        "poll_after": asdict(after),
    }


def offline_preflight(
    root: str | Path = PRODUCTION_ROOT,
    *,
    sheet_id: str = SHEET_ID,
    require_runtime_auth: bool = False,
) -> dict[str, Any]:
    """Instantiate concrete runtime constructors without making any external call."""
    previous_token = os.environ.get("CRYPTO_SHEETS_ACCESS_TOKEN")
    previous_file = os.environ.get("CRYPTO_SHEETS_SERVICE_ACCOUNT_FILE")
    if not require_runtime_auth:
        os.environ["CRYPTO_SHEETS_ACCESS_TOKEN"] = "offline-preflight-placeholder-not-a-credential"
        os.environ.pop("CRYPTO_SHEETS_SERVICE_ACCOUNT_FILE", None)
    service = None
    try:
        # Both concrete auth-owning transport classes are constructed. Their request
        # methods are never invoked, so the placeholder cannot leave this process.
        service = build_service_composition(root, sheet_id=sheet_id)
        return {
            "verified": True,
            "network_requests": 0,
            "model_calls": 0,
            "runtime_auth_required": require_runtime_auth,
            "composition": dict(service.composition),
            "owned_ranges": dict(OWNED_RANGES),
            "paths": {
                "root": str(service.paths.root),
                "intake_db": str(service.paths.intake_db),
                "dynamic_publication_db": str(service.paths.dynamic_publication_db),
                "publisher_root": str(service.paths.publisher_root),
                "effective_artifacts": list(service.paths.effective_artifacts),
            },
            "constructors": [
                type(service.queue).__name__, type(service.research).__name__,
                type(service.publisher).__name__, type(service.consumer).__name__,
                type(service.intake_sheets).__name__, GoogleSheetsTransport.__name__,
                DynamicGoogleSheetsTransport.__name__,
            ],
        }
    finally:
        if service is not None:
            service.close()
        if not require_runtime_auth:
            if previous_token is None:
                os.environ.pop("CRYPTO_SHEETS_ACCESS_TOKEN", None)
            else:
                os.environ["CRYPTO_SHEETS_ACCESS_TOKEN"] = previous_token
            if previous_file is None:
                os.environ.pop("CRYPTO_SHEETS_SERVICE_ACCOUNT_FILE", None)
            else:
                os.environ["CRYPTO_SHEETS_SERVICE_ACCOUNT_FILE"] = previous_file


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("preflight", "weekly", "manual", "intake"))
    parser.add_argument("--root", type=Path, default=PRODUCTION_ROOT)
    parser.add_argument("--config", type=Path)
    parser.add_argument("--run-id")
    parser.add_argument("--sheet-id", default=SHEET_ID)
    parser.add_argument("--require-runtime-auth", action="store_true")
    args = parser.parse_args(argv)

    config_path = args.config or args.root / "config/weekly.json"
    config = validate_release_config(json.loads(config_path.read_text(encoding="utf-8")))
    if args.action == "preflight":
        result = offline_preflight(
            args.root,
            sheet_id=args.sheet_id,
            require_runtime_auth=args.require_runtime_auth,
        )
        result["release_config_enabled"] = config["enabled"]
        print(json.dumps(result, ensure_ascii=False, sort_keys=True))
        return 0
    if args.action == "weekly":
        if not config["enabled"]:
            parser.error("weekly release config remains disabled")
        from .weekly_runner import main as weekly_main
        command = ["run", "--root", str(args.root)]
        if args.run_id:
            command += ["--run-id", args.run_id]
        return weekly_main(command)
    if args.action == "manual":
        from .manual_sync import main as manual_main
        return manual_main([
            "--root", str(args.root), "--sheet-id", args.sheet_id,
            "--budget-seconds", "120",
        ])

    service = build_service_composition(args.root, sheet_id=args.sheet_id)
    try:
        result = run_intake_once(service)
    except Exception as exc:
        result = {"verified": False, "stage": "blocked", "reason": type(exc).__name__}
        code = 3
    else:
        result["verified"] = True
        code = 0
    finally:
        service.close()
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return code


if __name__ == "__main__":
    raise SystemExit(main())
