"""WeKnora Python SDK — typed async client for the WeKnora REST API."""

from .client import WeKnoraClient
from .auth import Auth
from .errors import WeKnoraError, NotFoundError, UnauthorizedError
from .types import (
    KnowledgeBase,
    KnowledgeBaseInput,
    SearchRequest,
    SearchResponse,
    AskRequest,
    AskResponse,
    Automation,
    AutomationInput,
    AutomationTriggerType,
    AutomationActionType,
    Database,
    DatabaseInput,
    Row,
    FormulaEvalRequest,
    FormulaEvalResponse,
    CollabDoc,
    CollabDocFile,
    Agent,
    AgentRun,
    Connector,
    VerificationReport,
)

__version__ = "0.7.27"

__all__ = [
    "WeKnoraClient",
    "Auth",
    "WeKnoraError",
    "NotFoundError",
    "UnauthorizedError",
    "KnowledgeBase",
    "KnowledgeBaseInput",
    "SearchRequest",
    "SearchResponse",
    "AskRequest",
    "AskResponse",
    "Automation",
    "AutomationInput",
    "AutomationTriggerType",
    "AutomationActionType",
    "Database",
    "DatabaseInput",
    "Row",
    "FormulaEvalRequest",
    "FormulaEvalResponse",
    "CollabDoc",
    "CollabDocFile",
    "Agent",
    "AgentRun",
    "Connector",
    "VerificationReport",
]
