"""Pydantic models mirroring the OpenAPI specification."""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class _Base(BaseModel):
    model_config = ConfigDict(extra="allow", populate_by_name=True)


class KnowledgeBaseType(str, Enum):
    WIKI = "wiki"
    RAG = "rag"
    HYBRID = "hybrid"


class KnowledgeBase(_Base):
    id: str | None = None
    name: str
    description: str | None = None
    type: KnowledgeBaseType
    chunk_count: int | None = None
    created_at: datetime | None = None
    updated_at: datetime | None = None


class KnowledgeBaseInput(_Base):
    name: str
    description: str | None = None
    type: KnowledgeBaseType


class KnowledgeBasePatch(_Base):
    name: str | None = None
    description: str | None = None


class KnowledgeBasePage(_Base):
    items: list[KnowledgeBase]
    next_page_token: str | None = None


class SearchRequest(_Base):
    query: str
    top_k: int = 10
    rerank: bool = True
    filter: dict[str, Any] | None = None


class SearchHit(_Base):
    chunk_id: str
    score: float
    text: str
    document_id: str
    document_title: str
    highlights: list[str] | None = None


class SearchResponse(_Base):
    hits: list[SearchHit]


class Citation(_Base):
    chunk_id: str
    document_title: str
    text: str
    score: float


class AskRequest(_Base):
    question: str
    conversation_id: str | None = None
    stream: bool = False


class AskResponse(_Base):
    answer: str
    citations: list[Citation]
    conversation_id: str | None = None


class ChatMessage(_Base):
    role: str  # user / assistant / system
    content: str


class ChatRequest(_Base):
    messages: list[ChatMessage]
    conversation_id: str | None = None


class ChatChunkType(str, Enum):
    DELTA = "delta"
    CITATION = "citation"
    DONE = "done"
    ERROR = "error"


class ChatChunk(_Base):
    type: ChatChunkType
    content: str | None = None
    citation: Citation | None = None
    error: str | None = None


class DatabaseColumn(_Base):
    id: str | None = None
    name: str
    type: str
    options: dict[str, Any] | None = None


class DatabaseView(_Base):
    id: str | None = None
    name: str
    type: str


class Database(_Base):
    id: str | None = None
    kb_id: str | None = None
    name: str
    columns: list[DatabaseColumn] | None = None
    views: list[DatabaseView] | None = None


class DatabaseInput(_Base):
    name: str
    columns: list[DatabaseColumn]


class Row(_Base):
    id: str | None = None
    database_id: str | None = None
    values: dict[str, Any]
    created_at: datetime | None = None
    updated_at: datetime | None = None


class FormulaEvalRequest(_Base):
    expression: str
    context: dict[str, Any] | None = None


class FormulaEvalResponse(_Base):
    value: Any
    type: str | None = None
    error: str | None = None


class AutomationTriggerType(str, Enum):
    MANUAL = "manual"
    SCHEDULED = "scheduled"
    ROW_CHANGED = "row_changed"
    WEBHOOK = "webhook"


class AutomationActionType(str, Enum):
    UPDATE_FIELD = "update_field"
    CREATE_ROW = "create_row"
    SEND_WEBHOOK = "send_webhook"
    RUN_AGENT = "run_agent"
    NOTIFY = "notify"


class AutomationStep(_Base):
    id: str
    action_type: AutomationActionType
    config: dict[str, Any] | None = None
    next_ids: list[str] | None = None


class Automation(_Base):
    id: str | None = None
    tenant_id: str | None = None
    kb_id: str | None = None
    database_id: str
    name: str
    trigger_type: AutomationTriggerType
    trigger_config: dict[str, Any] | None = None
    steps: list[AutomationStep]
    enabled: bool | None = None
    created_at: datetime | None = None
    updated_at: datetime | None = None


class AutomationInput(_Base):
    database_id: str
    name: str
    trigger_type: AutomationTriggerType
    trigger_config: dict[str, Any] | None = None
    steps: list[AutomationStep]
    enabled: bool | None = True


class CollabDoc(_Base):
    id: str | None = None
    kb_id: str | None = None
    title: str
    kind: str
    created_at: datetime | None = None
    updated_at: datetime | None = None
    current_version: int | None = None


class CollabDocInput(_Base):
    kb_id: str
    title: str
    kind: str


class CollabDocFile(_Base):
    doc_id: str
    version: int
    sha256: str
    size_bytes: int
    content_type: str
    created_at: datetime | None = None


class Agent(_Base):
    id: str | None = None
    tenant_id: str | None = None
    name: str
    description: str | None = None
    model: str
    tools: list[str] | None = None
    memory: str | None = None
    system_prompt: str | None = None


class AgentRun(_Base):
    id: str | None = None
    agent_id: str | None = None
    status: str | None = None
    triggered_by: str | None = None
    input: dict[str, Any] | None = None
    output: dict[str, Any] | None = None
    steps_count: int | None = None
    tokens_used: int | None = None
    started_at: datetime | None = None
    finished_at: datetime | None = None


class Connector(_Base):
    id: str | None = None
    tenant_id: str | None = None
    kind: str
    name: str
    status: str | None = None
    last_sync_at: datetime | None = None
    config: dict[str, Any] | None = None


class VerificationReport(_Base):
    kb_id: str | None = None
    scanned_at: datetime | None = None
    trust_score: float | None = None
    findings: list[dict[str, Any]] | None = None
