import pytest
from weknora import (
    WeKnoraClient,
    Auth,
    UnauthorizedError,
    NotFoundError,
    KnowledgeBaseInput,
    KnowledgeBaseType,
    SearchRequest,
    FormulaEvalRequest,
    AutomationInput,
    AutomationTriggerType,
    AutomationActionType,
    AutomationStep,
)


class FakeAsyncClient:
    def __init__(self, responses):
        self.responses = list(responses)
        self.calls = []

    async def request(self, method, url, **kwargs):
        self.calls.append((method, url, kwargs))
        status, body = self.responses.pop(0)
        return _FakeResponse(status, body)

    async def aclose(self):
        pass


class _FakeResponse:
    def __init__(self, status, body):
        self.status_code = status
        self._body = body
        self.content = body.encode() if isinstance(body, str) else body
        self.text = body if isinstance(body, str) else body.decode()
        self.reason_phrase = ""

    def json(self):
        import json
        return json.loads(self._body)


@pytest.mark.asyncio
async def test_create_kb():
    fake = FakeAsyncClient([(201, '{"id":"kb-1","name":"Eng","type":"rag"}')])
    c = WeKnoraClient(base_url="https://x.test/v1", auth=Auth.bearer("t"), client=fake)
    kb = await c.create_kb(KnowledgeBaseInput(name="Eng", type=KnowledgeBaseType.RAG))
    assert kb.id == "kb-1"
    assert fake["calls"][0][0] == "POST"
    await c.aclose()


@pytest.mark.asyncio
async def test_not_found_raises():
    fake = FakeAsyncClient([(404, '{"error":"kb not found"}')])
    c = WeKnoraClient(base_url="https://x.test/v1", auth=Auth.api_key("k"), client=fake)
    with pytest.raises(NotFoundError):
        await c.get_kb("missing")
    await c.aclose()


@pytest.mark.asyncio
async def test_unauthorized_raises():
    fake = FakeAsyncClient([(401, '{"error":"bad token"}')])
    c = WeKnoraClient(base_url="https://x.test/v1", auth=Auth.bearer("wrong"), client=fake)
    with pytest.raises(UnauthorizedError):
        await c.get_kb("any")
    await c.aclose()


@pytest.mark.asyncio
async def test_eval_formula():
    fake = FakeAsyncClient([(200, '{"value":110,"type":"number"}')])
    c = WeKnoraClient(base_url="https://x.test/v1", auth=Auth.bearer("t"), client=fake)
    out = await c.eval_formula("kb-1", FormulaEvalRequest(expression="price*1.1", context={"price": 100}))
    assert out.value == 110
    await c.aclose()


@pytest.mark.asyncio
async def test_create_automation():
    fake = FakeAsyncClient([(201, '{"id":"auto-1","database_id":"db-1","name":"send","trigger_type":"row_changed","steps":[]}')])
    c = WeKnoraClient(base_url="https://x.test/v1", auth=Auth.bearer("t"), client=fake)
    auto = await c.create_automation("kb-1", AutomationInput(
        database_id="db-1",
        name="send",
        trigger_type=AutomationTriggerType.ROW_CHANGED,
        steps=[AutomationStep(id="s1", action_type=AutomationActionType.NOTIFY)],
    ))
    assert auto.id == "auto-1"
    await c.aclose()


def test_requires_base_url_and_auth():
    import pytest
    with pytest.raises(ValueError):
        WeKnoraClient(base_url="", auth=Auth.bearer("x"))
    with pytest.raises(ValueError):
        WeKnoraClient(base_url="https://x", auth=None)
