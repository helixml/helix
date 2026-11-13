"""Drives Helix's Exa-compatible API with the unmodified exa-py SDK.

Usage: HELIX_URL=http://localhost:8080 HELIX_API_KEY=... python exa_py_test.py
"""
import os
import sys
from urllib.parse import urlparse

from exa_py import Exa

base_url = os.environ["HELIX_URL"].rstrip("/") + "/api/v1/websearch"
exa = Exa(api_key=os.environ["HELIX_API_KEY"], base_url=base_url)
failures = []


def check(name, cond, detail=""):
    print(("PASS " if cond else "FAIL ") + name + (f" — {detail}" if detail else ""))
    if not cond:
        failures.append(name)


r = exa.search("go generics tutorial", num_results=3)
check("search returns results", len(r.results) > 0, f"{len(r.results)} results")
check("search results have id/url/title", all(x.id and x.url and x.title is not None for x in r.results))

r = exa.search_and_contents(
    "go generics tutorial", num_results=3, text={"max_characters": 500}, highlights=True, summary=True
)
check("search_and_contents text", all(x.text for x in r.results))
check("search_and_contents highlights", all(x.highlights for x in r.results))
check("search_and_contents summary", all(x.summary for x in r.results))

ids = [x.id for x in r.results]
c = exa.get_contents(ids[:1], text=True)
check("get_contents by id", len(c.results) == 1 and c.results[0].text, c.results[0].url if c.results else "")
c = exa.get_contents(["https://go.dev/doc/tutorial/generics"], text={"max_characters": 200})
check("get_contents by url", len(c.results) == 1 and len(c.results[0].text) <= 200)

s = exa.find_similar("https://go.dev/doc/tutorial/generics", num_results=3, exclude_source_domain=True)
check("find_similar", len(s.results) > 0 and all(urlparse(x.url).hostname != "go.dev" for x in s.results))

a = exa.answer("In which Go version were generics introduced?")
check("answer", "1.18" in str(a.answer), str(a.answer)[:80])
check("answer citations", len(a.citations) > 0)

chunks = list(exa.stream_answer("In which Go version were generics introduced?"))
text = "".join(ch.content or "" for ch in chunks)
check("stream_answer", "1.18" in text, f"{len(chunks)} chunks")
check("stream_answer citations", any(ch.citations for ch in chunks))

try:
    exa.search("x", num_results=500)
    check("invalid request raises", False)
except Exception as e:  # exa-py raises ValueError for any >= 400
    check("invalid request raises", "400" in str(e), str(e)[:100])

sys.exit(1 if failures else 0)
