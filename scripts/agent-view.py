#!/usr/bin/env python3
"""Readable live view of a Cursor agent round (stream-json on stdin).

    scripts/agent-loop.sh                     # shows this view while it runs
    tail -n +1 -f ~/Library/Logs/broadwave-agent/round-0007.jsonl | scripts/agent-view.py
"""
import json
import sys
import time

DIM, BOLD, RED, GREEN, YELLOW, BLUE, CYAN, RESET = (
    "\033[2m", "\033[1m", "\033[31m", "\033[32m", "\033[33m", "\033[34m", "\033[36m", "\033[0m",
)
if not sys.stdout.isatty():
    DIM = BOLD = RED = GREEN = YELLOW = BLUE = CYAN = RESET = ""

WIDTH = 160


def clip(text, n=WIDTH):
    text = " ".join(str(text).split())
    return text if len(text) <= n else text[: n - 1] + "…"


def stamp():
    return time.strftime("%H:%M:%S")


def describe(call):
    """Name and the one argument that matters for a tool call."""
    kind = next((k for k in call if k.endswith("ToolCall")), "tool")
    args = (call.get(kind) or {}).get("args", {}) if isinstance(call.get(kind), dict) else {}
    name = kind.removesuffix("ToolCall")
    for key in ("command", "description", "path", "filePath", "targetFile", "globPattern",
                "pattern", "query", "url", "prompt", "subagentType", "toolName"):
        if args.get(key):
            value = args[key]
            if key == "prompt":
                value = args.get("description") or value
            return name, value
    return name, clip(json.dumps(args), 100) if args else ""


def outcome(call):
    kind = next((k for k in call if k.endswith("ToolCall")), None)
    result = (call.get(kind) or {}).get("result") if kind else None
    if not isinstance(result, dict):
        return ""
    if "success" not in result:
        key = next(iter(result), "failed")
        return f"{RED}{key}{RESET} {DIM}{clip(json.dumps(result.get(key)), 120)}{RESET}"
    success = result.get("success")
    if isinstance(success, dict):
        code = success.get("exitCode")
        if code not in (None, 0):
            return f"{YELLOW}exit {code}{RESET}"
    return ""


def main():
    thinking = []
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            print(f"{DIM}{stamp()} {clip(line)}{RESET}", flush=True)
            continue
        kind, sub = event.get("type"), event.get("subtype")
        if kind == "system" and sub == "init":
            print(f"{BOLD}{stamp()} ── round started · {event.get('model')} · session {event.get('session_id')}{RESET}", flush=True)
        elif kind == "thinking" and sub == "delta":
            thinking.append(event.get("text", ""))
        elif kind == "thinking" and sub == "completed":
            if thinking:
                print(f"{DIM}{stamp()} ∴ {clip(''.join(thinking), 200)}{RESET}", flush=True)
            thinking = []
        elif kind == "assistant":
            for part in event.get("message", {}).get("content", []):
                if part.get("type") == "text" and part.get("text", "").strip():
                    print(f"{CYAN}{stamp()} ▸ {part['text'].strip()}{RESET}", flush=True)
        elif kind == "tool_call" and sub == "started":
            name, arg = describe(event.get("tool_call", {}))
            color = BLUE if name in ("task", "Task") else ""
            print(f"{color}{stamp()}   {name:<10} {clip(arg, WIDTH - 24)}{RESET}", flush=True)
        elif kind == "tool_call" and sub == "completed":
            note = outcome(event.get("tool_call", {}))
            if note:
                print(f"{stamp()}   {' ':<10} {note}", flush=True)
        elif kind == "result":
            ok = not event.get("is_error")
            mins = event.get("duration_ms", 0) / 60000
            print(f"{GREEN if ok else RED}{BOLD}{stamp()} ── round ended ({'ok' if ok else 'error'}, {mins:.1f} min){RESET}", flush=True)
        elif kind == "error" or event.get("is_error"):
            print(f"{RED}{stamp()} ! {clip(json.dumps(event), 200)}{RESET}", flush=True)


if __name__ == "__main__":
    try:
        main()
    except (KeyboardInterrupt, BrokenPipeError):
        pass
