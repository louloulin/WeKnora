"""Sentinel error types for the WeKnora SDK."""


class WeKnoraError(Exception):
    """Base class for all WeKnora SDK errors."""

    def __init__(self, message: str, status_code: int = 0, code: str = "") -> None:
        super().__init__(message)
        self.status_code = status_code
        self.code = code


class UnauthorizedError(WeKnoraError):
    """401 — the credentials were rejected."""


class ForbiddenError(WeKnoraError):
    """403 — the caller is not allowed to perform the operation."""


class NotFoundError(WeKnoraError):
    """404 — the resource does not exist."""


class RateLimitError(WeKnoraError):
    """429 — the caller exceeded the rate limit."""
