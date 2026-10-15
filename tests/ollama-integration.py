"""Live checks against the Compose compatibility endpoint; no downloads or tool execution."""

import json
import argparse
import subprocess
import time
import urllib.request


def chat(messages, tools=None):
    payload = {
        "model": "qwen38",
        "messages": messages,
        "stream": True,
        "reasoning_effort": "high",
        "max_tokens": 1024,
        "temperature": 0,
    }
    if tools:
        payload["tools"] = tools
    request = urllib.request.Request(
        "http://127.0.0.1:11434/v1/chat/completions",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
    )
    content, calls, finish, done = "", {}, None, False
    with urllib.request.urlopen(request, timeout=360) as response:
        for line in response:
            if not line.startswith(b"data: "):
                continue
            data = line[6:].strip()
            if data == b"[DONE]":
                done = True
                break
            chunk = json.loads(data)
            for choice in chunk.get("choices", []):
                delta = choice.get("delta", {})
                content += delta.get("content") or ""
                if choice.get("finish_reason"):
                    finish = choice["finish_reason"]
                for call in delta.get("tool_calls", []):
                    entry = calls.setdefault(
                        call["index"],
                        {"id": "", "type": "function", "function": {"name": "", "arguments": ""}},
                    )
                    entry["id"] += call.get("id") or ""
                    for key in ("name", "arguments"):
                        entry["function"][key] += call.get("function", {}).get(key) or ""
    assert done and finish, f"Stream did not terminate correctly: {finish=}, {done=}"
    assert finish != "length", "Model exhausted its output budget without completing"
    assert content.strip() or calls, "Model returned only reasoning with no usable answer"
    return {"role": "assistant", "content": content, **({"tool_calls": list(calls.values())} if calls else {})}


def main():
    history = [
        {"role": "system", "content": "Follow the user's instructions concisely."},
        {"role": "user", "content": "Remember the test label is BROCCOLI. Reply with that label only."},
    ]
    answer = chat(history)
    assert "BROCCOLI" in answer["content"], answer
    history.extend([
        answer,
        {"role": "assistant", "content": None, "reasoning": "An empty prior turn."},
        {"role": "user", "content": "What test label did I give you? Answer with the label only."},
    ])
    answer = chat(history)
    assert "BROCCOLI" in answer["content"], answer
    print("PASS: streamed reply, high-effort compatibility, null-history follow-up")

    tools = [{
        "type": "function",
        "function": {
            "name": "get_test_value",
            "description": "Return the current test value. Call this to obtain it.",
            "parameters": {"type": "object", "properties": {}, "required": []},
        },
    }]
    history.append({"role": "user", "content": "Call get_test_value now. Do not guess its value."})
    answer = chat(history, tools)
    assert answer.get("tool_calls"), answer
    history.append(answer)
    for call in answer["tool_calls"]:
        assert call["function"]["name"] == "get_test_value", call
        assert json.loads(call["function"]["arguments"]) == {}, call
        history.append({"role": "tool", "tool_call_id": call["id"], "content": "CANARY-742"})
    history.append({"role": "user", "content": "State the exact test value returned by the tool."})
    answer = chat(history, tools)
    assert "CANARY-742" in answer["content"], answer
    history.append(answer)
    print("PASS: streamed tool call and tool-result continuation")

    history.extend([
        {"role": "assistant", "content": None},
        {"role": "user", "content": "Summarize this conversation for a continuation. Preserve the label and tool result."},
    ])
    summary = chat(history)
    assert "BROCCOLI" in summary["content"] and "CANARY-742" in summary["content"], summary
    continuation = chat([
        {"role": "system", "content": "Prior conversation summary: " + summary["content"]},
        {"role": "user", "content": "Give the test label and the exact tool result from before compaction."},
    ])
    assert "BROCCOLI" in continuation["content"] and "CANARY-742" in continuation["content"], continuation
    print("PASS: summary request with null history and post-summary continuation")


def benchmark(context):
    """Fill most of the requested context and check retrieval, latency, and GPU residency."""
    marker = "CONTEXT-CANARY-927"
    filler = " filler" * (context - 16384)
    payload = {
        "model": "qwen38",
        "messages": [
            {"role": "system", "content": "Retrieve the exact validation marker. Do not summarize the filler."},
            {"role": "user", "content": f"Validation marker: {marker}\n{filler}\nReturn only the validation marker."},
        ],
        "think": "medium",
        "stream": False,
        "keep_alive": "30m",
        "options": {"num_ctx": context, "num_predict": 512, "temperature": 0},
    }
    request = urllib.request.Request(
        "http://127.0.0.1:11434/api/chat",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
    )
    started = time.monotonic()
    with urllib.request.urlopen(request, timeout=600) as response:
        result = json.load(response)
    assert marker in result["message"]["content"], result["message"]
    assert result["done_reason"] != "length", result["done_reason"]
    assert result["prompt_eval_count"] >= context - 17000, result["prompt_eval_count"]
    with urllib.request.urlopen("http://127.0.0.1:11434/api/ps", timeout=30) as response:
        models = json.load(response)["models"]
    assert len(models) == 1 and models[0]["context_length"] == context, models
    gpu = subprocess.check_output([
        "nvidia-smi", "--query-gpu=memory.used,memory.total", "--format=csv,noheader,nounits",
    ], text=True).strip()
    used, total = map(int, gpu.split(","))
    print(json.dumps({
        "context": context,
        "prompt_tokens": result["prompt_eval_count"],
        "elapsed_seconds": round(time.monotonic() - started, 2),
        "load_seconds": round(result["load_duration"] / 1e9, 2),
        "prompt_seconds": round(result["prompt_eval_duration"] / 1e9, 2),
        "gpu_used_mib": used,
        "gpu_free_mib": total - used,
        "model_size": models[0]["size"],
        "model_vram": models[0]["size_vram"],
        "answer": result["message"]["content"],
    }), flush=True)
    assert models[0]["size_vram"] >= models[0]["size"] * 0.95, "Significant CPU offload"
    assert total - used >= 2048, "Less than 2 GiB GPU headroom"


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--benchmark-context", type=int, choices=[65536, 81920, 98304])
    args = parser.parse_args()
    if args.benchmark_context:
        benchmark(args.benchmark_context)
    else:
        main()
