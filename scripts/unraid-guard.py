#!/usr/bin/env python3
"""PreToolUse guard: keeps the unattended agent inside Broadwave's corner of TUS.

The agent may manage the Broadwave containers (Broadwave-Staging, wg-lab-*, and
Broadwave in a phase deploy), their appdata, and the Broadwave template. It may
not stop, restart, remove, or reconfigure anything else on the Unraid box:
other containers (channelsdvr_intel, Plex, ...), the array, shares, plugins,
VMs, the flash drive, or the host itself. Read-only commands pass.

Wired up by ~/.grok/hooks/broadwave-unraid-guard.json. It prints a Grok/Claude
hook decision on stdout and only acts on sessions whose workspace is this repo.

    echo '{"toolInput":{"command":"ssh root@192.168.1.2 docker restart plex"},"cwd":"'$PWD'"}' | scripts/unraid-guard.py
"""
import json
import os
import re
import shlex
import sys

REPO = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))
LOG = os.path.expanduser("~/Library/Logs/broadwave-agent/guard.log")

TUS = re.compile(r"192\.168\.1\.2(?![0-9])|@tus\b|\btus:", re.I)
OURS = re.compile(r"^(Broadwave|Broadwave-Staging|wg-lab-[\w.-]+|broadwave[\w.:/-]*)$")
OUR_PATHS = (
    "/mnt/cache/appdata/broadwave",  # also covers broadwave-staging and broadwave-lab
    "/tmp/",
)
# docker verbs that change a container, and the ones that change the host.
CONTAINER_VERBS = r"stop|restart|kill|rm|start|pause|unpause|update|rename|exec|create|run|cp|commit|attach|wait"
HOST_BLOCK = [
    (r"\bdocker\s+(system|builder|volume|network|plugin|swarm|compose|context)\b", "docker host-level command"),
    (r"\bdocker(-compose)?\s+compose\b|\bdocker-compose\b", "docker compose"),
    (r"\b(image|container)\s+prune\b|\bprune\b", "prune"),
    (r"\b(reboot|shutdown|poweroff|halt|init\s+[06])\b", "host power"),
    (r"\b(mdcmd|mover|virsh|zpool|zfs|btrfs|mkfs\w*|fdisk|parted|wipefs|sgdisk|hdparm|smartctl\s+-[st])\b", "array/disk/VM tool"),
    (r"/etc/rc\.d/", "Unraid service script"),
    (r"\b(installplg|plugin\s+(install|remove|update)|/usr/local/sbin/(plugin|emhttp|update))", "Unraid plugin/system change"),
    (r"\b(user(add|del|mod)|passwd|chpasswd|crontab\s+-[er])\b", "host accounts/cron"),
    (r"\bkillall\b|\bpkill\b|\bkill\s+-?\d*\s*1\b", "killing host processes"),
    (r"\bchannels?dvr|\bplex\b|\bjellyfin\b|\bemby\b|\btdarr\b|\bsonarr\b|\bradarr\b|\bportkey\b", "another app on TUS"),
    (r"--privileged|--pid[= ]host|docker\.sock", "privileged container"),
]


def targets_tus(cmd):
    return bool(TUS.search(cmd)) and bool(re.search(r"\b(ssh|scp|rsync|sftp|curl|wget|http|nc)\b", cmd))


def remote_text(cmd):
    """The command plus any local script it feeds to ssh (ssh host bash -s < file)."""
    text = cmd
    for path in re.findall(r"<\s*([^\s|;&<>]+)", cmd):
        path = os.path.expanduser(path)
        if os.path.isfile(path) and os.path.getsize(path) < 1_000_000:
            try:
                text += "\n" + open(path, errors="replace").read()
            except OSError:
                pass
    return text


def check_docker(text):
    for m in re.finditer(rf"\bdocker\s+(?:container\s+)?({CONTAINER_VERBS})\b([^;&|\n]*)", text):
        verb, rest = m.group(1), m.group(2)
        if "$(" in rest or "`" in rest:
            return f"docker {verb} with a computed container list"
        try:
            words = shlex.split(rest, posix=True)
        except ValueError:
            words = rest.split()
        if verb in ("run", "create"):
            name = None
            for i, w in enumerate(words):
                if w == "--name" and i + 1 < len(words):
                    name = words[i + 1]
                elif w.startswith("--name="):
                    name = w.split("=", 1)[1]
            if not name or not OURS.match(name):
                return f"docker {verb} must name a Broadwave container (got {name!r})"
            for i, w in enumerate(words):
                if w in ("-v", "--volume", "--mount") and i + 1 < len(words):
                    src = words[i + 1].split(":", 1)[0].replace("$PWD", "/mnt/cache/appdata/broadwave")
                    ok = (src.startswith(OUR_PATHS) or src.startswith("/mnt/user/media/ota-recordings")
                          or src.startswith("/dev/dri") or not src.startswith("/"))
                    if not ok:
                        return f"docker {verb} mounts {src}, outside Broadwave's appdata"
            continue
        # First non-flag words are container names (exec/cp: just the first one).
        names = []
        skip = False
        for w in words:
            if skip:
                skip = False
                continue
            if w.startswith("-"):
                if w in ("-t", "--time", "-s", "--signal", "-u", "--user", "-w", "--workdir", "-e", "--env",
                         "--cpus", "--memory", "-m", "--restart"):
                    skip = True
                continue
            names.append(w)
            if verb in ("exec", "cp", "commit", "attach"):
                break
        if verb == "cp" and names:
            names = [names[0].split(":", 1)[0]] if ":" in names[0] else []
        for n in names:
            if not OURS.match(n):
                return f"docker {verb} on {n!r}, which is not a Broadwave container"
    for m in re.finditer(r"\bdocker\s+(?:image\s+)?(rmi|rm)\b([^;&|\n]*)", text):
        if m.group(0).split()[1] in ("rmi",) or "image" in m.group(0):
            for w in m.group(2).split():
                if not w.startswith("-") and "broadwave" not in w.lower():
                    return f"removing image {w!r}, which is not a Broadwave image"
    return None


def check_files(text):
    # Destructive file operations on TUS must stay inside Broadwave's appdata or /tmp.
    for m in re.finditer(r"\b(rm|mv|chmod|chown|truncate|shred|dd|tee|sed\s+-i|cp|ln|mkdir|rmdir|touch)\b([^;&|\n]*)", text):
        for path in re.findall(r"(/(?:mnt|boot|etc|usr|var|root|opt|lib|config)[^\s'\";|&]*)", m.group(2)):
            if path.startswith("/mnt/user/media/ota-recordings") and m.group(1) in ("rm", "mv", "truncate", "shred"):
                return f"{m.group(1)} on recordings ({path}); recordings are user data"
            if path.startswith("/boot/config/plugins/dockerMan/templates-user/my-Broadwave"):
                continue
            if not path.startswith(OUR_PATHS) and not path.startswith("/mnt/user/media/ota-recordings"):
                return f"{m.group(1)} touches {path}, outside Broadwave's appdata"
    for m in re.finditer(r">>?\s*(/[^\s'\";|&]+)", text):
        path = m.group(1)
        if not re.match(r"/(mnt|boot|etc|usr|var|root|opt|lib|config)/", path) or path.startswith(OUR_PATHS):
            continue
        if path.startswith("/var/folders/"):
            continue
        if path.startswith("/boot/config/plugins/dockerMan/templates-user/my-Broadwave"):
            continue
        return f"writes to {path}, outside Broadwave's appdata"
    # scp/rsync uploads land only in Broadwave's appdata.
    for m in re.finditer(r"(?:192\.168\.1\.2|tus):(/[^\s'\";|&]*)", text):
        path = m.group(1)
        if not (path.startswith(OUR_PATHS) or path.startswith("/boot/config/plugins/dockerMan/templates-user/my-Broadwave")):
            if re.search(r"\b(scp|rsync)\b", text) and not re.search(r"(?:192\.168\.1\.2|tus):" + re.escape(path) + r"\S*\s+\S", text):
                return f"uploads to {path}, outside Broadwave's appdata"
    return None


def check_http(cmd):
    # The Unraid web UI and other apps' APIs: read-only at most. Broadwave's own ports are fine.
    for m in re.finditer(r"\b(curl|wget|http)\b[^;&|\n]*", cmd):
        seg = m.group(0)
        if not TUS.search(seg):
            continue
        if re.search(r"192\.168\.1\.2:(8477|8490)\b", seg):
            continue
        if re.search(r"-X\s*(POST|PUT|PATCH|DELETE)|--data|-d\s|-F\s|--form|--upload|-T\s", seg):
            return "writes to a non-Broadwave service on TUS (web UI or another app)"
    return None


def decide(cmd):
    if not targets_tus(cmd):
        return None
    text = remote_text(cmd)
    for pattern, why in HOST_BLOCK:
        if re.search(pattern, text, re.I):
            return why
    return check_docker(text) or check_files(text) or check_http(cmd)


def main():
    try:
        event = json.load(sys.stdin)
    except Exception:
        return
    root = os.path.realpath(event.get("workspaceRoot") or event.get("cwd") or os.getcwd())
    if not (root == REPO or root.startswith(REPO + os.sep)) and os.environ.get("BROADWAVE_UNRAID_GUARD") != "1":
        return
    tool_input = event.get("toolInput") or event.get("tool_input") or {}
    cmd = tool_input.get("command") or tool_input.get("cmd") or ""
    if not isinstance(cmd, str) or not cmd:
        return
    why = decide(cmd)
    if not why:
        return
    reason = (f"Blocked by scripts/unraid-guard.py: {why}. On TUS you may only manage Broadwave-Staging, "
              "wg-lab-* containers, and Broadwave in a phase deploy, under /mnt/cache/appdata/broadwave*. "
              "Leave every other container, share, plugin, and host setting alone. If this is truly needed, "
              "write it in docs/plan/BLOCKERS.md for the owner and move on.")
    try:
        os.makedirs(os.path.dirname(LOG), exist_ok=True)
        with open(LOG, "a") as f:
            f.write(json.dumps({"ts": event.get("timestamp"), "why": why, "cmd": cmd[:2000]}) + "\n")
    except OSError:
        pass
    print(json.dumps({"decision": "deny", "reason": reason,
                      "hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "deny",
                                             "permissionDecisionReason": reason}}))


if __name__ == "__main__":
    main()
