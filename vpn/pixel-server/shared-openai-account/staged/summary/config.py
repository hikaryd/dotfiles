from __future__ import annotations

from pathlib import Path
from typing import Any
from zoneinfo import ZoneInfo

from pydantic import AliasChoices, Field, SecretStr, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Validated production configuration. Secret values never appear in repr/errors."""

    model_config = SettingsConfigDict(env_prefix="SUMMARY_BOT_", extra="ignore")
    source_chat_id: int
    delivery_chat_id: int | None = None
    accepted_private_chat_ids: tuple[int, ...] = ()
    database_url: SecretStr | None = None
    telegram_token: SecretStr | None = None
    gemini_api_key: SecretStr | None = None
    openai_api_key: SecretStr | None = None
    webhook_secret: SecretStr | None = None
    telegram_token_file: Path | None = Field(default=None, repr=False)
    status_telegram_token: SecretStr | None = Field(default=None, repr=False)
    status_telegram_token_file: Path | None = Field(default=None, repr=False)
    gemini_api_key_file: Path | None = Field(default=None, repr=False)
    openai_api_key_file: Path | None = Field(default=None, repr=False)
    database_url_file: Path | None = Field(default=None, repr=False)
    webhook_secret_file: Path | None = Field(default=None, repr=False)
    analytics_enabled: bool = Field(
        default=False, validation_alias=AliasChoices("PRODUCT_ANALYTICS_ENABLED", "analytics_enabled")
    )
    analytics_gateway_url: str | None = Field(
        default=None, validation_alias=AliasChoices("PRODUCT_ANALYTICS_GATEWAY_URL", "analytics_gateway_url")
    )
    analytics_service_token: SecretStr | None = Field(
        default=None,
        repr=False,
        validation_alias=AliasChoices("PRODUCT_ANALYTICS_CREDENTIAL", "analytics_service_token"),
    )
    analytics_service_token_file: Path | None = Field(
        default=None,
        repr=False,
        validation_alias=AliasChoices("PRODUCT_ANALYTICS_CREDENTIAL_FILE", "analytics_service_token_file"),
    )
    analytics_actor_key: SecretStr | None = Field(
        default=None, repr=False, validation_alias=AliasChoices("PRODUCT_ANALYTICS_HMAC_KEY", "analytics_actor_key")
    )
    analytics_actor_key_file: Path | None = Field(
        default=None,
        repr=False,
        validation_alias=AliasChoices("PRODUCT_ANALYTICS_HMAC_KEY_FILE", "analytics_actor_key_file"),
    )
    analytics_actor_key_id: str = Field(
        default="v1",
        pattern=r"^v[1-9][0-9]*$",
        validation_alias=AliasChoices("PRODUCT_ANALYTICS_ACTOR_KEY_ID", "analytics_actor_key_id"),
    )
    analytics_queue_capacity: int = Field(
        default=256,
        ge=1,
        le=4096,
        validation_alias=AliasChoices("PRODUCT_ANALYTICS_QUEUE_SIZE", "analytics_queue_capacity"),
    )
    user_api_id: int | None = Field(default=None, repr=False)
    user_api_hash: SecretStr | None = Field(default=None, repr=False)
    user_api_id_file: Path | None = Field(default=None, repr=False)
    user_api_hash_file: Path | None = Field(default=None, repr=False)
    user_session_file: Path | None = Field(default=None, repr=False)
    history_expected_chat_id: int | None = None
    history_page_size: int = Field(default=100, ge=1, le=500)
    history_batch_size: int = Field(default=100, ge=1, le=500)
    history_photo_max_bytes: int = Field(default=10_000_000, ge=1, le=20_000_000)
    history_max_messages: int = Field(default=5_000_000, ge=1, le=100_000_000)
    history_max_pages: int = Field(default=100_000, ge=1, le=10_000_000)
    history_max_elapsed_seconds: int = Field(default=86_400, ge=1, le=604_800)
    webhook_url: str | None = None
    timezone: str = "Europe/Moscow"
    retention_days: int = Field(default=7, ge=1)
    mode: str = Field(default="bakeoff", pattern="^(bakeoff|production)$")
    production_provider: str = Field(default="gemini", pattern="^(gemini|openai|codex)$")
    gemini_model: str = "gemini-3.7-flash"
    openai_model: str = "gpt-5.6-luna"
    openai_heavy_model: str | None = None
    codex_api_fallback_enabled: bool = True
    codex_strict_account: bool = False
    codex_model: str = "gpt-5.6-terra"
    codex_heavy_model: str = "gpt-5.6-sol"
    pipeline_max_workers: int = Field(default=4, ge=1, le=8)
    humor_enabled: bool = True
    production_model: str | None = None
    provider_timeout_seconds: float = Field(default=120, gt=0)
    formation_hour: int = Field(default=21, ge=0, le=23)
    formation_minute: int = Field(default=40, ge=0, le=59)
    schedule_hour: int = Field(default=22, ge=0, le=23)
    schedule_minute: int = Field(default=0, ge=0, le=59)
    report_day_offset_days: int = Field(default=1, ge=0, le=1)
    pipeline_version: str = "v1"
    telegram_max_messages: int = Field(default=2, ge=1, le=2)
    telegram_message_limit: int = Field(default=4000, ge=500, le=4096)
    telegram_max_file_bytes: int = Field(default=20_000_000, ge=1)
    telegram_timeout_seconds: float = Field(default=20, gt=0)
    telegram_max_retries: int = Field(default=3, ge=0, le=10)
    media_directory: Path = Path("/var/lib/summary-bot/media")
    poll_timeout_seconds: int = Field(default=30, ge=0, le=50)
    pin_group_delivery: bool = False
    alertmanager_url: str | None = None
    status_chat_id: int | None = None
    status_message_thread_id: int | None = Field(default=None, ge=1)
    app_metrics_host: str = "127.0.0.1"
    app_metrics_port: int = Field(default=9100, ge=0, le=65535)
    scheduler_metrics_host: str = "127.0.0.1"
    scheduler_metrics_port: int = Field(default=9101, ge=0, le=65535)
    poller_metrics_host: str = "127.0.0.1"
    poller_metrics_port: int = Field(default=9102, ge=0, le=65535)

    @field_validator("source_chat_id")
    @classmethod
    def source_is_supergroup(cls, value: int) -> int:
        if value >= 0 or not str(value).startswith("-100"):
            raise ValueError("source_chat_id must be a negative Telegram supergroup ID (-100…)")
        return value

    @field_validator("accepted_private_chat_ids")
    @classmethod
    def private_ids_are_positive(cls, value: tuple[int, ...]) -> tuple[int, ...]:
        if any(item <= 0 for item in value):
            raise ValueError("accepted_private_chat_ids must contain positive private chat IDs")
        return value

    @model_validator(mode="before")
    @classmethod
    def secret_files(cls, data: Any) -> Any:
        values = dict(data or {})
        canonical = {
            "PRODUCT_ANALYTICS_ENABLED": "analytics_enabled",
            "PRODUCT_ANALYTICS_GATEWAY_URL": "analytics_gateway_url",
            "PRODUCT_ANALYTICS_CREDENTIAL": "analytics_service_token",
            "PRODUCT_ANALYTICS_CREDENTIAL_FILE": "analytics_service_token_file",
            "PRODUCT_ANALYTICS_HMAC_KEY": "analytics_actor_key",
            "PRODUCT_ANALYTICS_HMAC_KEY_FILE": "analytics_actor_key_file",
            "PRODUCT_ANALYTICS_ACTOR_KEY_ID": "analytics_actor_key_id",
            "PRODUCT_ANALYTICS_QUEUE_SIZE": "analytics_queue_capacity",
        }
        for external, internal in canonical.items():
            if external in values and internal not in values:
                values[internal] = values.pop(external)
        # pydantic-settings supplies environment keys by field name. Explicit value wins.
        for name in (
            "telegram_token",
            "status_telegram_token",
            "gemini_api_key",
            "openai_api_key",
            "database_url",
            "webhook_secret",
            "user_api_hash",
            "analytics_service_token",
            "analytics_actor_key",
        ):
            file_value = values.pop(f"{name}_file", None)
            if file_value and not values.get(name):
                try:
                    values[name] = Path(str(file_value)).read_text(encoding="utf-8").strip()
                except OSError as exc:
                    raise ValueError(f"cannot read configured {name}_file") from exc
        api_id_file = values.pop("user_api_id_file", None)
        if api_id_file and not values.get("user_api_id"):
            try:
                values["user_api_id"] = int(Path(str(api_id_file)).read_text(encoding="utf-8").strip())
            except (OSError, ValueError) as exc:
                raise ValueError("cannot read configured user_api_id_file") from exc
        return values

    @model_validator(mode="after")
    def delivery_policy(self) -> Settings:
        if self.database_url is None:
            raise ValueError("database_url or database_url_file is required")
        if not self.database_url.get_secret_value().startswith(("postgresql://", "postgres://")):
            raise ValueError("database_url must use PostgreSQL")
        if self.delivery_chat_id is None:
            self.delivery_chat_id = self.source_chat_id if self.mode == "production" else None
        if self.delivery_chat_id is None:
            raise ValueError("delivery_chat_id is required in bakeoff mode")
        if self.mode == "bakeoff" and self.delivery_chat_id not in self.accepted_private_chat_ids:
            raise ValueError("bakeoff delivery_chat_id must be an explicitly accepted private chat ID")
        if (
            self.mode == "production"
            and self.delivery_chat_id != self.source_chat_id
            and self.delivery_chat_id not in self.accepted_private_chat_ids
        ):
            raise ValueError("production delivery must target source_chat_id or an accepted private chat")
        if self.pin_group_delivery and (self.mode != "production" or self.delivery_chat_id != self.source_chat_id):
            raise ValueError("pin_group_delivery requires production delivery to source_chat_id")
        selected_model = {
            "gemini": self.gemini_model,
            "openai": self.openai_model,
            "codex": self.codex_model,
        }[self.production_provider]
        if self.production_model is None:
            self.production_model = selected_model
        elif self.production_model != selected_model:
            raise ValueError("production_model must match the selected provider model")
        if self.history_batch_size > self.history_page_size:
            raise ValueError("history_batch_size cannot exceed history_page_size")
        if self.history_expected_chat_id is not None and self.history_expected_chat_id != self.source_chat_id:
            raise ValueError("history_expected_chat_id must exactly match source_chat_id")
        if self.mode == "production" and self.timezone != "Europe/Moscow":
            raise ValueError("production timezone must be Europe/Moscow")
        if self.mode == "production" and (self.formation_hour, self.formation_minute) != (21, 40):
            raise ValueError("production formation contract is 21:40 Europe/Moscow")
        if self.mode == "production" and (self.schedule_hour, self.schedule_minute) != (22, 0):
            raise ValueError("production publication contract is 22:00 Europe/Moscow")
        if self.analytics_enabled:
            from summary_bot.analytics import validate_gateway_url

            if not self.analytics_gateway_url:
                raise ValueError("analytics_gateway_url is required when analytics is enabled")
            validate_gateway_url(self.analytics_gateway_url)
            if not self.secret("analytics_service_token"):
                raise ValueError("analytics_service_token is required when analytics is enabled")
            actor_key = self.secret("analytics_actor_key")
            if actor_key is None or len(actor_key.encode()) < 32:
                raise ValueError("analytics_actor_key must contain at least 32 bytes")
        return self

    @property
    def tz(self) -> ZoneInfo:
        return ZoneInfo(self.timezone)

    def secret(self, name: str) -> str | None:
        value = getattr(self, name)
        return value.get_secret_value() if isinstance(value, SecretStr) else None
