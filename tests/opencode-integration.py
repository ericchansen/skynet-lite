"""Check the running OpenCode desktop service using isolated, local-only sessions."""

import argparse
import base64
import json
from pathlib import Path
import urllib.parse
import urllib.request


def main(url):
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme != "http" or parsed.hostname not in ("127.0.0.1", "::1"):
        raise ValueError("Use the running service's loopback HTTP URL")
    password = json.loads(
        (Path.home() / ".config" / "opencode" / "service.json").read_text()
    )["password"]
    authorization = base64.b64encode(f"opencode:{password}".encode()).decode()

    def api(path, body=None):
        request = urllib.request.Request(
            url.rstrip("/") + path,
            data=None if body is None else json.dumps(body).encode(),
            headers={
                "Authorization": "Basic " + authorization,
                "Content-Type": "application/json",
            },
        )
        with urllib.request.urlopen(request, timeout=600) as response:
            raw = response.read()
        return json.loads(raw) if raw else None

    model = api("/api/model/default")["data"]
    assert model["providerID"] == "ollama" and model["id"] == "qwen38:latest", model["id"]
    assert model["limit"] == {"context": 81920, "input": 65536, "output": 8192}, model["limit"]
    assert model["body"]["reasoning_effort"] == "medium"
    assert model["compatibility"]["requireFinishReason"] is True
    assert model["settings"]["baseURL"] == "http://127.0.0.1:11434/v1"
    models = api("/api/model")["data"]
    named_qwen = [
        item["id"]
        for item in models
        if item["providerID"] == "ollama"
        and item["name"].startswith("Qwen3.8 27B abliterated Q6_K")
    ]
    assert named_qwen == ["qwen38:latest"], named_qwen
    plugins = api("/api/plugin")["data"]
    assert any(
        plugin["id"] == "skynet.compaction" and plugin["state"]["status"] == "active"
        for plugin in plugins
    ), "The concise compaction hook is not active"
    print("PASS: live OpenCode default, token budgets, reasoning, and strict streaming", flush=True)

    session = api(
        "/api/session",
        {
            "title": "Local Qwen tool and compaction verification",
            "location": {"directory": str(Path(__file__).resolve().parents[1])},
            "model": {"providerID": "ollama", "id": "qwen38:latest", "variant": "medium"},
        },
    )["data"]["id"]
    print("Verification session:", session, flush=True)
    path = f"/api/session/{session}"

    def prompt(text):
        api(path + "/prompt", {"text": text})
        api(f"/api/experimental/session/{session}/wait", {})
        messages = api(path + "/context")["data"]
        answers = [message for message in messages if message["type"] == "assistant"]
        assert answers, "OpenCode produced no assistant response"
        answer = answers[-1]
        assert not answer.get("error"), answer.get("error")
        assert answer.get("finish") not in (None, "error", "length"), answer.get("finish")
        return messages, answer

    messages, answer = prompt(
        "Use your file-reading tool to read compat/go.mod. Reply with exactly the module name. "
        "Do not run shell commands, modify files, or use network tools."
    )
    assert "skynet-lite/compat" in json.dumps(answer["content"]), answer["content"]
    tool_parts = [
        part
        for message in messages
        if message["type"] == "assistant"
        for part in message["content"]
        if part["type"] == "tool"
    ]
    assert tool_parts, "No actual tool call was recorded"
    print("PASS: OpenCode file tool, tool-result continuation, and completed streaming", flush=True)

    marker = "OPENCODE-CONTEXT-927"
    api(
        path + "/synthetic",
        {
            "text": (
                f"Synthetic verification data only. Preserve this marker: {marker}.\n"
                + " filler" * 35000
                + f"\nThe important marker remains {marker}."
            ),
            "resume": False,
        },
    )
    messages, answer = prompt(
        "Return the exact OPENCODE-CONTEXT marker from the preceding synthetic data. "
        "Do not use tools or modify files."
    )
    compactions = [message for message in messages if message["type"] == "compaction"]
    assert compactions, "Automatic compaction did not run"
    compacted = compactions[-1]
    assert compacted["status"] == "completed", compacted.get("error")
    assert compacted["reason"] == "auto", compacted["reason"]
    assert compacted["model"]["variant"] == "none", compacted["model"]
    assert compacted["tokens"]["output"] < 8192, compacted["tokens"]
    assert len(compacted["summary"].split()) <= 1200, "Compaction summary is too verbose"
    assert marker in json.dumps(answer["content"]), answer["content"]
    print("PASS: automatic compaction and marker-preserving continuation", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", required=True, help="Running OpenCode service loopback URL")
    main(parser.parse_args().url)
