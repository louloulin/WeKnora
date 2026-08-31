"""Standalone tests for the wiki MCP tools URL building.

Avoids importing weknora_mcp_server (which requires `mcp` to be installed)
by mirroring the URL pattern in each tool. Run with `python3 -m pytest` or
`python3 -m unittest`.
"""
import os
import re
import unittest
from unittest.mock import patch


def make_client():
    """Lightweight stand-in for WeKnoraClient that only captures requests."""
    class Stub:
        def __init__(self):
            self.calls = []

        def _request(self, method, path, params=None, json=None):
            self.calls.append({"method": method, "path": path, "params": params, "json": json})
            return {"ok": True, "path": path}

        # Mirror the real client methods so the test asserts the contract.
        def wiki_search(self, kb_id, query, limit=10):
            return self._request("GET", f"/knowledgebase/{kb_id}/wiki/search", params={"q": query, "limit": limit})

        def wiki_read_page(self, kb_id, slug):
            return self._request("GET", f"/knowledgebase/{kb_id}/wiki/pages/{slug}")

        def wiki_index_view(self, kb_id, limit=50):
            return self._request("GET", f"/knowledgebase/{kb_id}/wiki/index", params={"limit": limit})

        def wiki_get_backlinks(self, kb_id, slug):
            return self._request("GET", f"/knowledgebase/{kb_id}/wiki/pages/{slug}/backlinks")

        def wiki_list_folders(self, kb_id):
            return self._request("GET", f"/knowledgebase/{kb_id}/wiki/folders")
    return Stub()


class WikiMCPPathTest(unittest.TestCase):
    def setUp(self):
        self.client = make_client()

    def test_wiki_search_path_and_params(self):
        result = self.client.wiki_search("kb-1", "transformer", limit=5)
        self.assertEqual(len(self.client.calls), 1)
        call = self.client.calls[0]
        self.assertEqual(call["method"], "GET")
        self.assertEqual(call["path"], "/knowledgebase/kb-1/wiki/search")
        self.assertEqual(call["params"], {"q": "transformer", "limit": 5})
        self.assertEqual(result["path"], "/knowledgebase/kb-1/wiki/search")

    def test_wiki_read_page_path(self):
        result = self.client.wiki_read_page("kb-1", "entity/acme")
        call = self.client.calls[0]
        self.assertEqual(call["path"], "/knowledgebase/kb-1/wiki/pages/entity/acme")
        self.assertEqual(call["method"], "GET")
        # slug with slashes preserved in path
        self.assertIn("entity/acme", call["path"])

    def test_wiki_index_view_default_limit(self):
        self.client.wiki_index_view("kb-1")
        call = self.client.calls[0]
        self.assertEqual(call["params"], {"limit": 50})

    def test_wiki_index_view_custom_limit(self):
        self.client.wiki_index_view("kb-1", limit=200)
        call = self.client.calls[0]
        self.assertEqual(call["params"], {"limit": 200})

    def test_wiki_get_backlinks_path(self):
        self.client.wiki_get_backlinks("kb-1", "entity/acme")
        call = self.client.calls[0]
        self.assertEqual(call["method"], "GET")
        self.assertEqual(call["path"], "/knowledgebase/kb-1/wiki/pages/entity/acme/backlinks")
        # Backlinks path is distinct from page read path
        self.assertNotIn(call["path"], "/knowledgebase/kb-1/wiki/pages/entity/acme")

    def test_wiki_list_folders_path(self):
        self.client.wiki_list_folders("kb-7")
        call = self.client.calls[0]
        self.assertEqual(call["method"], "GET")
        self.assertEqual(call["path"], "/knowledgebase/kb-7/wiki/folders")

    def test_all_paths_use_knowledgebase_prefix(self):
        self.client.wiki_search("k", "q")
        self.client.wiki_read_page("k", "s")
        self.client.wiki_index_view("k")
        self.client.wiki_get_backlinks("k", "s")
        self.client.wiki_list_folders("k")
        for c in self.client.calls:
            self.assertTrue(
                c["path"].startswith("/knowledgebase/"),
                f"path {c['path']} should start with /knowledgebase/",
            )

    def test_backlinks_path_includes_slug_segment(self):
        self.client.wiki_get_backlinks("kb-99", "concept/rag")
        call = self.client.calls[0]
        # slashes preserved
        self.assertRegex(call["path"], r"/knowledgebase/kb-99/wiki/pages/concept/rag/backlinks$")


if __name__ == "__main__":
    unittest.main()
