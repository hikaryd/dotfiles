from __future__ import annotations

import signal
from collections.abc import Callable
from datetime import datetime, timedelta
from threading import Event
from typing import cast

import httpx

from summary_bot.analytics import Analytics, AnalyticsConfig, BoundedAnalytics, NullAnalytics
from summary_bot.config import Settings
from summary_bot.ingestion import IngestionService, Poller
from summary_bot.metrics import InMemoryMetrics
from summary_bot.providers import (
    BoundedFallbackProvider,
    CodexSubscriptionAdapter,
    GeminiAdapter,
    HermesImageAdapter,
    OpenAIAdapter,
    StageProvider,
)
from summary_bot.scheduler import (
    _SCHEDULED_CAPABILITY,
    DailyScheduler,
    publish_scheduled_day,
    run_formation_day,
)
from summary_bot.status import NullStatusReporter, StatusReporter, TelegramTopicStatusReporter
from summary_bot.storage import PostgresRepository
from summary_bot.telegram import TelegramTransport


def analytics_from_settings(settings: Settings) -> Analytics:
    if not settings.analytics_enabled:
        return NullAnalytics()
    return BoundedAnalytics(
        AnalyticsConfig(
            enabled=True,
            gateway_url=settings.analytics_gateway_url,
            service_token=settings.analytics_service_token,
            actor_key=settings.analytics_actor_key,
            actor_key_id=settings.analytics_actor_key_id,
            queue_capacity=settings.analytics_queue_capacity,
        )
    )


def telegram_from_settings(settings: Settings) -> TelegramTransport:
    token = settings.secret("telegram_token")
    if token is None:
        raise ValueError("telegram_token or telegram_token_file is required")
    client = httpx.Client(base_url="https://api.telegram.org/", timeout=settings.telegram_timeout_seconds)
    return TelegramTransport(token, client, max_retries=settings.telegram_max_retries)


def status_telegram_from_settings(settings: Settings, fallback: TelegramTransport) -> TelegramTransport:
    """Send lifecycle statuses from the same public identity as the Summary Bot."""
    # Keep the legacy setting readable for rollback-compatible configuration,
    # but never let it change the user-visible sender identity.
    return fallback


def providers_from_settings(settings: Settings, metrics: InMemoryMetrics | None = None) -> list[StageProvider]:
    if settings.codex_strict_account and (
        settings.mode != "production" or settings.production_provider != "codex" or settings.codex_api_fallback_enabled
    ):
        raise ValueError("Strict Codex account requires production Codex with API fallback disabled")
    if (
        settings.mode == "production"
        and settings.production_provider == "codex"
        and not settings.codex_api_fallback_enabled
    ):
        return [
            CodexSubscriptionAdapter(
                model_id=settings.codex_model,
                heavy_model_id=settings.codex_heavy_model,
                turn_timeout=settings.provider_timeout_seconds,
                strict_account=settings.codex_strict_account,
            )
        ]
    configured: dict[str, StageProvider] = {}
    if key := settings.secret("gemini_api_key"):
        configured["gemini"] = GeminiAdapter(
            key, httpx.Client(timeout=settings.provider_timeout_seconds), model_id=settings.gemini_model
        )
    if key := settings.secret("openai_api_key"):
        configured["openai"] = OpenAIAdapter(
            key,
            httpx.Client(timeout=settings.provider_timeout_seconds),
            model_id=settings.openai_model,
            heavy_model_id=settings.openai_heavy_model,
        )
    configured["codex"] = CodexSubscriptionAdapter(
        model_id=settings.codex_model,
        heavy_model_id=settings.codex_heavy_model,
        turn_timeout=settings.provider_timeout_seconds,
    )
    if settings.mode == "bakeoff":
        if not {"gemini", "openai"} <= set(configured):
            raise ValueError("bakeoff mode requires both Gemini and OpenAI API keys")
        return [configured["gemini"], configured["openai"]]
    provider = configured.get(settings.production_provider)
    if provider is None:
        raise ValueError(f"production provider {settings.production_provider!r} is not configured")
    if settings.production_provider == "codex":
        fallback = configured.get("openai")
        if fallback is None or fallback.model_id != "gpt-5.6-luna":
            raise ValueError("Codex production requires the configured OpenAI gpt-5.6-luna stage fallback")
        provider = cast(StageProvider, BoundedFallbackProvider(provider, fallback, metrics=metrics))
    return [provider]


def image_provider_from_settings(settings: Settings) -> StageProvider:
    if getattr(settings, "codex_strict_account", False):
        if (
            settings.mode != "production"
            or settings.production_provider != "codex"
            or settings.codex_api_fallback_enabled
        ):
            raise ValueError("Strict Codex account requires production Codex with API fallback disabled")
        return HermesImageAdapter(
            httpx.Client(
                timeout=httpx.Timeout(min(settings.provider_timeout_seconds, 120), connect=10),
                trust_env=False,
                follow_redirects=False,
            )
        )
    key = settings.secret("openai_api_key")
    if not key:
        raise ValueError("OpenAI API key is required for Luna image OCR")
    return OpenAIAdapter(
        key,
        httpx.Client(timeout=settings.provider_timeout_seconds),
        model_id="gpt-5.6-luna",
        heavy_model_id=None,
    )


def scheduler_from_settings(
    settings: Settings, *, deliver: bool = True, run_namespace: str = "preview", metrics: InMemoryMetrics | None = None
) -> DailyScheduler:
    runtime_metrics = metrics or InMemoryMetrics()
    repository = PostgresRepository(settings.secret("database_url") or "", settings.retention_days)
    telegram = telegram_from_settings(settings) if deliver else None
    status_reporter: StatusReporter
    if not deliver:
        status_reporter = NullStatusReporter()
    elif run_namespace == "private-preview":
        status_chat_id = settings.delivery_chat_id
        if (
            status_chat_id is None
            or status_chat_id <= 0
            or status_chat_id not in settings.accepted_private_chat_ids
            or settings.pin_group_delivery
        ):
            raise ValueError("private-preview requires an allowlisted private destination with pinning disabled")
        status_telegram = status_telegram_from_settings(settings, cast(TelegramTransport, telegram))
        status_reporter = TelegramTopicStatusReporter(
            status_telegram,
            status_chat_id,
            banner="🧪 ТЕСТОВЫЙ ПРОГОН\nВ публичный чат ничего не отправляется.",
        )
    elif run_namespace != "production":
        status_reporter = NullStatusReporter()
    else:
        status_chat_id = 464089905
        if status_chat_id not in settings.accepted_private_chat_ids:
            raise ValueError("production lifecycle status recipient 464089905 must be an accepted private chat ID")
        status_telegram = status_telegram_from_settings(settings, cast(TelegramTransport, telegram))
        status_reporter = TelegramTopicStatusReporter(status_telegram, status_chat_id)
    providers = providers_from_settings(settings)
    for provider in providers:
        if isinstance(provider, BoundedFallbackProvider):
            provider.metrics = runtime_metrics
    return DailyScheduler(
        settings,
        repository,
        providers,
        telegram,
        image_provider_from_settings(settings),
        status_reporter,
        run_namespace,
        runtime_metrics,
        analytics_from_settings(settings),
    )


def poller_from_settings(settings: Settings) -> tuple[Poller, TelegramTransport]:
    repository = PostgresRepository(settings.secret("database_url") or "", settings.retention_days)
    telegram = telegram_from_settings(settings)
    service = IngestionService(
        repository,
        settings.source_chat_id,
        telegram,
        settings.telegram_max_file_bytes,
        analytics_from_settings(settings),
    )
    return Poller(service, repository, telegram, settings.poll_timeout_seconds, InMemoryMetrics()), telegram


def install_stop_handlers(stop: Event) -> None:
    def request_stop(_signum: int, _frame: object) -> None:
        stop.set()

    signal.signal(signal.SIGTERM, request_stop)
    signal.signal(signal.SIGINT, request_stop)


def run_poll_loop(poller: Poller, stop: Event, *, once: bool = False) -> None:
    while not stop.is_set():
        poller.poll_once()
        if once:
            return


def seconds_until_schedule(now: datetime, hour: int, minute: int) -> tuple[float, datetime]:
    target = now.replace(hour=hour, minute=minute, second=0, microsecond=0)
    if target <= now:
        target += timedelta(days=1)
    return (target - now).total_seconds(), target


def next_schedule_event(now: datetime, settings: Settings) -> tuple[datetime, str]:
    formation = now.replace(hour=settings.formation_hour, minute=settings.formation_minute, second=0, microsecond=0)
    publication = now.replace(hour=settings.schedule_hour, minute=settings.schedule_minute, second=0, microsecond=0)
    if now < formation:
        return formation, "formation"
    if now < publication:
        return now, "formation"
    if now < publication + timedelta(minutes=1):
        return now, "publication"
    return formation + timedelta(days=1), "formation"


def run_scheduler_loop(
    settings: Settings,
    scheduler: DailyScheduler,
    stop: Event,
    *,
    now: Callable[[], datetime] | None = None,
    wait: Callable[[float], bool] | None = None,
) -> None:
    clock = now or (lambda: datetime.now(settings.tz))
    waiter = wait or stop.wait
    while not stop.is_set():
        current = clock()
        if isinstance(scheduler, DailyScheduler):
            scheduler.repository.monitoring_touch("idle", current)
        if not isinstance(scheduler, DailyScheduler):
            delay, target = seconds_until_schedule(current, settings.schedule_hour, settings.schedule_minute)
            event = "legacy"
        else:
            target, event = next_schedule_event(current, settings)
            if event == "formation":
                candidate_day = (target - timedelta(days=settings.report_day_offset_days)).date()
                candidate_key = scheduler.run_key(candidate_day)
                if scheduler.repository.run_status(candidate_key) == "ready":
                    publication = current.replace(
                        hour=settings.schedule_hour,
                        minute=settings.schedule_minute,
                        second=0,
                        microsecond=0,
                    )
                    if current < publication:
                        target, event = publication, "publication"
            delay = max(0.0, (target - current).total_seconds())
        if wait is None and delay > 30:
            if waiter(30):
                return
            continue
        if waiter(delay):
            return
        invocation_time = clock()
        scheduler_metrics = getattr(scheduler, "metrics", None)
        if scheduler_metrics is not None:
            lag_target = target
            if event == "publication":
                lag_target = invocation_time.replace(
                    hour=settings.schedule_hour, minute=settings.schedule_minute, second=0, microsecond=0
                )
            elif event == "formation":
                lag_target = invocation_time.replace(
                    hour=settings.formation_hour, minute=settings.formation_minute, second=0, microsecond=0
                )
            scheduler_metrics.observe(
                "tool_action_duration_seconds",
                max(0.0, (invocation_time - lag_target).total_seconds()),
                labels={"action": "scheduler", "outcome": "succeeded"},
            )
        report_day = (target - timedelta(days=settings.report_day_offset_days)).date()
        run_key = scheduler.run_key(report_day) if isinstance(scheduler, DailyScheduler) else None
        try:
            try:
                if isinstance(scheduler, DailyScheduler):
                    if event == "formation":
                        status = scheduler.repository.run_status(run_key or "")
                        may_form = status != "failed" or scheduler.repository.claim_continuation(run_key or "")
                        if may_form:
                            run_formation_day(scheduler, report_day, _SCHEDULED_CAPABILITY, invocation_time)
                        if scheduler.repository.run_status(run_key or "") == "ready":
                            completion = clock().astimezone(settings.tz)
                            cutoff = completion.replace(
                                hour=settings.schedule_hour,
                                minute=settings.schedule_minute,
                                second=0,
                                microsecond=0,
                            )
                            if completion >= cutoff:
                                scheduler.repository.finish_run(
                                    run_key, "failed", "formation missed the 22:00 publication cutoff"
                                )
                    else:
                        publish_scheduled_day(scheduler, report_day, _SCHEDULED_CAPABILITY, invocation_time)
                else:
                    scheduler.run_day(report_day)
            except Exception as exc:
                if run_key is not None and scheduler.repository.run_status(run_key) == "pending":
                    scheduler.repository.finish_run(run_key, "failed", f"scheduler: {type(exc).__name__}")
            if event == "formation" and run_key is not None and scheduler.repository.claim_continuation(run_key):
                try:
                    run_formation_day(scheduler, report_day, _SCHEDULED_CAPABILITY, clock())
                except Exception as exc:
                    if scheduler.repository.run_status(run_key) in {"pending", "recovering"}:
                        scheduler.repository.finish_run(
                            run_key, "failed", f"scheduler continuation: {type(exc).__name__}"
                        )
                if scheduler.repository.run_status(run_key) in {"pending", "recovering"}:
                    scheduler.repository.finish_run(run_key, "failed", "scheduler continuation incomplete")
        finally:
            scheduler.repository.cleanup(datetime.now().astimezone())
        if wait is not None:
            return
