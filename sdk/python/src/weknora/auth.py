"""Auth helpers for the WeKnora SDK."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Union


@dataclass(frozen=True)
class BearerAuth:
    """JWT bearer token authentication."""

    token: str

    def apply(self, headers: dict[str, str]) -> None:
        headers["Authorization"] = f"Bearer {self.token}"


@dataclass(frozen=True)
class APIKeyAuth:
    """X-API-Key authentication for service-to-service calls."""

    key: str

    def apply(self, headers: dict[str, str]) -> None:
        headers["X-API-Key"] = self.key


Auth = Union[BearerAuth, APIKeyAuth]


def bearer(token: str) -> Auth:
    return BearerAuth(token=token)


def api_key(key: str) -> Auth:
    return APIKeyAuth(key=key)
