"""WeKnora Python SDK — async client backed by httpx."""

from __future__ import annotations

import json
from typing import Any, AsyncIterator

import httpx

from .auth import APIKeyAuth, Auth, BearerAuth
from .errors import (
    ForbiddenError,
    NotFoundError,
    RateLimitError,
    UnauthorizedError,
    WeKnoraError,
)
from .types import (
    Agent,
    AgentRun,
    AskRequest,
    AskResponse,
    Automation,
    AutomationInput,
    ChatRequest,
    ChatChunk,
    CollabDoc,
    CollabDocFile,
    CollabDocInput,
    Connector,
    Database,
    DatabaseInput,
    FormulaEvalRequest,
    FormulaEvalResponse,
    KnowledgeBase,
    KnowledgeBaseInput,
    KnowledgeBasePage,
    KnowledgeBasePatch,
    Row,
    SearchRequest,
    SearchResponse,
    VerificationReport,
    VerificationRequest,
)


class WeKnoraClient:
    """Async client for the WeKnora Enterprise Knowledge Platform."""

    def __init__(
        self,
        base_url: str,
        auth: Auth,
        *,
        timeout: float = 30.0,
        max_retries: int = 3,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        if not base_url:
            raise ValueError("WeKnora: base_url is required")
        if auth is None:
            raise ValueError("WeKnora: auth is required")
        self.base_url = base_url.rstrip("/")
        self._auth = auth
        self._max_retries = max_retries
        self._owns_client = client is None
        self._client = client or httpx.AsyncClient(timeout=timeout)

    # ------------------------------------------------------------------ HTTP
    async def _request(
        self,
        method: str,
        path: str,
        body: Any = None,
        query: dict[str, Any] | None = None,
    ) -> Any:
        headers: dict[str, str] = {}
        self._auth.apply(headers)
        json_body = json.dumps(body) if body is not None else None
        last_err: Exception | None = None
        for attempt in range(max(self._max_retries, 1)):
            try:
                resp = await self._client.request(
                    method,
                    f"{self.base_url}{path}",
                    headers=headers,
                    content=json_body,
                    params=query,
                )
            except httpx.HTTPError as err:
                last_err = err
                await self._sleep_backoff(attempt)
                continue
            if 200 <= resp.status_code < 300:
                if resp.status_code == 204:
                    return None
                if not resp.content:
                    return None
                return resp.json()
            err = self._to_error(resp)
            if isinstance(err, (UnauthorizedError, ForbiddenError)):
                raise err
            if resp.status_code < 500 and resp.status_code != 429:
                raise err
            last_err = err
            await self._sleep_backoff(attempt)
        if last_err:
            raise last_err
        raise WeKnoraError("WeKnora: request failed without an error")

    @staticmethod
    def _to_error(resp: httpx.Response) -> WeKnoraError:
        message = resp.text
        try:
            payload = resp.json()
            if isinstance(payload, dict) and "error" in payload:
                message = str(payload["error"])
        except Exception:
            pass
        if resp.status_code == 401:
            return UnauthorizedError(message, resp.status_code, "unauthorized")
        if resp.status_code == 403:
            return ForbiddenError(message, resp.status_code, "forbidden")
        if resp.status_code == 404:
            return NotFoundError(message, resp.status_code, "not_found")
        if resp.status_code == 429:
            return RateLimitError(message, resp.status_code, "rate_limited")
        return WeKnoraError(message, resp.status_code, resp.reason_phrase or "")

    @staticmethod
    async def _sleep_backoff(attempt: int) -> None:
        import asyncio
        await asyncio.sleep(min(0.2 * (2 ** attempt), 2.0))

    async def aclose(self) -> None:
        if self._owns_client:
            await self._client.aclose()

    # ----------------------------------------------------------- Stream API
    async def stream_chat(self, kb_id: str, req: ChatRequest) -> AsyncIterator[ChatChunk]:
        headers: dict[str, str] = {"Accept": "application/x-ndjson"}
        self._auth.apply(headers)
        async with self._client.stream(
            "POST",
            f"{self.base_url}/knowledge-bases/{kb_id}/chat",
            headers=headers,
            content=json.dumps(req.model_dump(exclude_none=True)),
        ) as resp:
            if resp.status_code != 200:
                raise self._to_error(resp)
            buffer = ""
            async for chunk in resp.aiter_text():
                buffer += chunk
                while "\n" in buffer:
                    line, buffer = buffer.split("\n", 1)
                    line = line.strip()
                    if line:
                        yield ChatChunk.model_validate_json(line)

    # ------------------------------------------------------------ KnowledgeBase
    async def create_kb(self, input: KnowledgeBaseInput) -> KnowledgeBase:
        data = await self._request("POST", "/knowledge-bases", body=input.model_dump(exclude_none=True))
        return KnowledgeBase.model_validate(data)

    async def get_kb(self, kb_id: str) -> KnowledgeBase:
        data = await self._request("GET", f"/knowledge-bases/{kb_id}")
        return KnowledgeBase.model_validate(data)

    async def list_kbs(self, page_size: int = 20, page_token: str | None = None) -> KnowledgeBasePage:
        data = await self._request(
            "GET",
            "/knowledge-bases",
            query={"page_size": page_size, "page_token": page_token} if page_token else {"page_size": page_size},
        )
        return KnowledgeBasePage.model_validate(data)

    async def update_kb(self, kb_id: str, patch: KnowledgeBasePatch) -> KnowledgeBase:
        data = await self._request("PATCH", f"/knowledge-bases/{kb_id}", body=patch.model_dump(exclude_none=True))
        return KnowledgeBase.model_validate(data)

    async def delete_kb(self, kb_id: str) -> None:
        await self._request("DELETE", f"/knowledge-bases/{kb_id}")

    # ------------------------------------------------------------ Search / Chat
    async def search(self, kb_id: str, req: SearchRequest) -> SearchResponse:
        data = await self._request("POST", f"/knowledge-bases/{kb_id}/search", body=req.model_dump(exclude_none=True))
        return SearchResponse.model_validate(data)

    async def ask(self, kb_id: str, req: AskRequest) -> AskResponse:
        data = await self._request("POST", f"/knowledge-bases/{kb_id}/ask", body=req.model_dump(exclude_none=True))
        return AskResponse.model_validate(data)

    # ------------------------------------------------------------ Database / Formula
    async def eval_formula(self, kb_id: str, req: FormulaEvalRequest) -> FormulaEvalResponse:
        data = await self._request("POST", f"/knowledge-bases/{kb_id}/formula/eval", body=req.model_dump(exclude_none=True))
        return FormulaEvalResponse.model_validate(data)

    async def create_database(self, kb_id: str, input: DatabaseInput) -> Database:
        data = await self._request("POST", f"/knowledge-bases/{kb_id}/databases", body=input.model_dump(exclude_none=True))
        return Database.model_validate(data)

    async def insert_rows(self, kb_id: str, database_id: str, rows: list[dict[str, Any]]) -> list[Row]:
        data = await self._request("POST", f"/knowledge-bases/{kb_id}/databases/{database_id}/rows", body={"rows": rows})
        return [Row.model_validate(r) for r in data]

    # ------------------------------------------------------------ Automation
    async def create_automation(self, kb_id: str, input: AutomationInput) -> Automation:
        data = await self._request(
            "POST",
            f"/knowledge-bases/{kb_id}/databases/{input.database_id}/automations",
            body=input.model_dump(exclude_none=True),
        )
        return Automation.model_validate(data)

    async def run_automation(self, kb_id: str, automation_id: str, payload: dict[str, Any] | None = None) -> dict[str, Any]:
        return await self._request(
            "POST",
            f"/knowledge-bases/{kb_id}/automations/{automation_id}/run",
            body=payload,
        )

    # ------------------------------------------------------------ Verification
    async def verify(self, kb_id: str, req: VerificationRequest) -> VerificationReport:
        data = await self._request("POST", f"/knowledge-bases/{kb_id}/verify", body=req.model_dump(exclude_none=True))
        return VerificationReport.model_validate(data)


# Convenience aliases
client = WeKnoraClient
