#!/usr/bin/env python3
"""Readable live view of an agent round: Cursor's stream-json or Grok's streaming-json on stdin.

    scripts/agent-loop.sh                     # shows this view while it runs
    tail -n +1 -f ~/Library/Logs/broadwave-agent/round-*-0007.jsonl | scripts/agent-view.py
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


GROK_ARG_KEYS = ("command", "description", "path", "file_path", "target_file", "pattern", "query",
                 "url", "prompt", "task", "name")


class Grok:
    """Grok streams text and thoughts as small deltas; print each run once it ends."""

    def __init__(self):
        self.kind, self.buf, self.started = None, [], False

    def flush(self):
        text = "".join(self.buf).strip()
        if text and self.kind == "thought":
            print(f"{DIM}{stamp()} ∴ {clip(text, 200)}{RESET}", flush=True)
        elif text:
            print(f"{CYAN}{stamp()} ▸ {clip(text, 600)}{RESET}", flush=True)
        self.kind, self.buf = None, []

    def handle(self, event):
        kind = event.get("type")
        if not self.started:
            self.started = True
            print(f"{BOLD}{stamp()} ── round started · grok{RESET}", flush=True)
        if kind in ("text", "thought"):
            if self.kind != kind:
                self.flush()
                self.kind = kind
            self.buf.append(str(event.get("data", "")))
            return
        self.flush()
        if kind == "tool_call":
            raw = event.get("rawInput") or {}
            name = event.get("toolName") or event.get("title") or "tool"
            arg = next((raw[k] for k in GROK_ARG_KEYS if isinstance(raw, dict) and raw.get(k)), "")
            if not arg and raw:
                arg = clip(json.dumps(raw), 100)
            short = {"run_terminal_command": "shell", "read_file": "read", "search_replace": "edit",
                     "spawn_subagent": "task", "list_dir": "ls"}.get(name, name)
            color = BLUE if short == "task" else ""
            print(f"{color}{stamp()}   {short:<10} {clip(arg, WIDTH - 24)}{RESET}", flush=True)
        elif kind == "tool_call_update" and event.get("status") == "failed":
            parts = event.get("content") or []
            msg = next((p.get("content", {}).get("text") for p in parts if isinstance(p, dict)), "") or "failed"
            print(f"{stamp()}   {' ':<10} {RED if 'denied' in msg.lower() else YELLOW}{clip(msg, 140)}{RESET}", flush=True)
        elif kind == "end":
            ok = event.get("stopReason") in ("end_turn", None)
            print(f"{GREEN if ok else RED}{BOLD}{stamp()} ── round ended ({event.get('stopReason')}, "
                  f"{event.get('num_turns', '?')} turns){RESET}", flush=True)
        elif kind == "error":
            print(f"{RED}{stamp()} ! {clip(event.get('message') or json.dumps(event), 200)}{RESET}", flush=True)
        elif kind and kind.startswith(("auto_compact", "max_turns")):
            print(f"{YELLOW}{stamp()} · {kind}{RESET}", flush=True)


GROK_TYPES = {"text", "thought", "tool_call", "tool_call_update", "usage", "plan", "available_commands", "end"}


def main():
    grok = None
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
        if grok or (kind in GROK_TYPES and "subtype" not in event) or (kind or "").startswith(("auto_compact", "max_turns")):
            grok = grok or Grok()
            grok.handle(event)
            continue
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
    if grok:
        grok.flush()


if __name__ == "__main__":
    try:
        main()
    except (KeyboardInterrupt, BrokenPipeError):
        pass
