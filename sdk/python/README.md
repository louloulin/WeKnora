# weknora (Python SDK)

Official async Python SDK for the WeKnora Enterprise Knowledge Platform.

## Install

```bash
pip install weknora
```

## Quick start

```python
import asyncio
from weknora import WeKnoraClient, Auth

async def main():
    client = WeKnoraClient(
        base_url="https://api.weknora.com/v1",
        auth=Auth.bearer("MY_TOKEN"),
    )
    kb = await client.create_kb(name="Engineering", type="rag")
    hits = await client.search(kb.id, query="vector search")
    for hit in hits.hits:
        print(hit.document_title, hit.score)
    await client.aclose()

asyncio.run(main())
```

## Run tests

```bash
pip install -e .[dev]
pytest -q
```

## License

Apache-2.0
PY_EOF

echo "Python SDK done"
find sdk/python -type f | wc -l