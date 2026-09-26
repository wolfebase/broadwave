#!/usr/bin/env bash
# Regenerate the App Store screenshots from the in-app demo and upload them.
#
#   scripts/appstore-shots.sh
#
# Uses the Broadwave Shots simulators (iPhone 17 Pro Max, iPad Pro 13-inch M5,
# Apple TV 4K at 1080p). Each device captures Home, Guide, Watch, and
# Multiview from -BroadwaveDemo, then writes JPEGs with no alpha.
# A version that is still in review is not modified.
#
# SKIP_BUILD=1 skips xcodebuild when apple/build already has the apps.
# SKIP_UPLOAD=1 captures only.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/.evidence/as3"
ASC="${ASC:-$HOME/.blitz/bin/asc}"
APP_ID="6815795649"
mkdir -p "$OUT/iphone" "$OUT/ipad" "$OUT/tv"

if [[ "${SKIP_BUILD:-}" != "1" ]]; then
  make -C "$ROOT" apple
fi

IOS_APP="$(find "$ROOT/apple/build/dd" -path '*Debug-iphonesimulator/Broadwave.app' -type d | head -1)"
TV_APP="$(find "$ROOT/apple/build/dd" -path '*Debug-appletvsimulator/BroadwaveTV.app' -type d | head -1)"
[[ -d "$IOS_APP" && -d "$TV_APP" ]]

ensure_sim() {
  local name="$1" devtype="$2" runtime="$3"
  local udid
  udid="$(python3 - "$name" <<'PY'
import json, subprocess, sys
name = sys.argv[1]
data = json.loads(subprocess.check_output(["xcrun", "simctl", "list", "devices", "-j"]))
for devs in data["devices"].values():
    for dev in devs:
        if dev["name"] == name and dev["isAvailable"]:
            print(dev["udid"])
            raise SystemExit
PY
)"
  if [[ -z "$udid" ]]; then
    udid="$(xcrun simctl create "$name" "$devtype" "$runtime")"
    echo "created $name $udid" >&2
  fi
  printf '%s\n' "$udid"
}

IPHONE="$(ensure_sim "Broadwave Shots iPhone" \
  "com.apple.CoreSimulator.SimDeviceType.iPhone-17-Pro-Max" \
  "com.apple.CoreSimulator.SimRuntime.iOS-26-5")"
IPAD="$(ensure_sim "Broadwave Shots iPad" \
  "com.apple.CoreSimulator.SimDeviceType.iPad-Pro-13-inch-M5-12GB" \
  "com.apple.CoreSimulator.SimRuntime.iOS-26-5")"
TV="$(ensure_sim "Broadwave Shots TV" \
  "com.apple.CoreSimulator.SimDeviceType.Apple-TV-4K-3rd-generation-1080p" \
  "com.apple.CoreSimulator.SimRuntime.tvOS-26-5")"

boot_one() {
  local udid="$1"
  xcrun simctl boot "$udid" 2>/dev/null || true
  xcrun simctl bootstatus "$udid" -b
  # 9:41, full battery. tvOS ignores this override.
  xcrun simctl status_bar "$udid" override \
    --time "9:41" \
    --dataNetwork wifi \
    --wifiMode active \
    --cellularMode active \
    --batteryState charged \
    --batteryLevel 100 >/dev/null 2>&1 || true
}

shot() {
  local udid="$1" app="$2" dest="$3" wait_s="$4"
  shift 4
  xcrun simctl terminate "$udid" com.wolfeup.broadwave >/dev/null 2>&1 || true
  xcrun simctl spawn "$udid" defaults delete com.wolfeup.broadwave >/dev/null 2>&1 || true
  xcrun simctl launch "$udid" com.wolfeup.broadwave \
    -ApplePersistenceIgnoreState YES \
    -BroadwaveDemo YES \
    "$@" >/dev/null
  sleep "$wait_s"
  xcrun simctl io "$udid" screenshot --type=jpeg "$dest"
}

capture_device() {
  local udid="$1" app="$2" dir="$3"
  boot_one "$udid"
  xcrun simctl install "$udid" "$app"
  shot "$udid" "$app" "$dir/01-home.jpg" "${HOME_WAIT:-10}"
  shot "$udid" "$app" "$dir/02-guide.jpg" "${GUIDE_WAIT:-10}" -BroadwaveTab guide
  shot "$udid" "$app" "$dir/03-watch.jpg" "${WATCH_WAIT:-16}" -BroadwaveWatch 1 -BroadwaveInfo YES
  shot "$udid" "$app" "$dir/04-multiview.jpg" "${MV_WAIT:-16}" -BroadwaveMultiview 1,3
  xcrun simctl terminate "$udid" com.wolfeup.broadwave >/dev/null 2>&1 || true
  # Leave the staging simulators booted. Shut this shots device.
  xcrun simctl shutdown "$udid" >/dev/null 2>&1 || true
}

capture_device "$IPHONE" "$IOS_APP" "$OUT/iphone"
capture_device "$IPAD" "$IOS_APP" "$OUT/ipad"
capture_device "$TV" "$TV_APP" "$OUT/tv"

python3 - "$OUT" <<'PY'
import subprocess, sys
from pathlib import Path
root = Path(sys.argv[1])
allowed = {
    "iphone": {(1260, 2736), (1290, 2796), (1320, 2868), (2736, 1260), (2796, 1290), (2868, 1320)},
    "ipad": {(2048, 2732), (2064, 2752), (2732, 2048), (2752, 2064)},
    "tv": {(1920, 1080), (3840, 2160)},
}
for device, sizes in allowed.items():
    files = sorted((root / device).glob("*.jpg"))
    if len(files) != 4:
        raise SystemExit(f"{device} has {len(files)} shots")
    for path in files:
        info = subprocess.check_output(["sips", "-g", "format", "-g", "pixelWidth", "-g", "pixelHeight", str(path)], text=True)
        if "format: jpeg" not in info:
            subprocess.check_call(["sips", "-s", "format", "jpeg", "-s", "formatOptions", "90", str(path), "--out", str(path)])
            info = subprocess.check_output(["sips", "-g", "format", "-g", "pixelWidth", "-g", "pixelHeight", str(path)], text=True)
        width = height = None
        for line in info.splitlines():
            if "pixelWidth" in line:
                width = int(line.split()[-1])
            if "pixelHeight" in line:
                height = int(line.split()[-1])
        if (width, height) not in sizes:
            raise SystemExit(f"{path.name} is {width}x{height}, not a {device} store size")
        if path.stat().st_size < 40_000:
            raise SystemExit(f"{path} is only {path.stat().st_size} bytes")
        print(f"{device} {path.name} {width}x{height} {path.stat().st_size}")
PY

if [[ "${SKIP_UPLOAD:-}" == "1" ]]; then
  echo "captured under $OUT"
  exit 0
fi

python3 - "$ASC" "$APP_ID" "$OUT" <<'PY'
import json, subprocess, sys, urllib.request, time
from pathlib import Path
asc, app_id, out = sys.argv[1], sys.argv[2], sys.argv[3]
home = Path.home()
creds = json.loads((home / ".blitz/asc-credentials.json").read_text())
import jwt
now = int(time.time())
token = jwt.encode(
    {"iss": creds["issuerId"], "iat": now, "exp": now + 600, "aud": "appstoreconnect-v1"},
    creds["privateKey"], algorithm="ES256", headers={"kid": creds["keyId"], "typ": "JWT"},
)

def api(path):
    req = urllib.request.Request(
        "https://api.appstoreconnect.apple.com" + path,
        headers={"Authorization": f"Bearer {token}"},
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        return json.load(resp)

versions = api(f"/v1/apps/{app_id}/appStoreVersions?limit=10")
locked = {"WAITING_FOR_REVIEW", "IN_REVIEW", "PENDING_DEVELOPER_RELEASE", "PENDING_APPLE_RELEASE", "READY_FOR_SALE", "PROCESSING_FOR_DISTRIBUTION"}
want = {
    "IOS": [("iphone", "IPHONE_67"), ("ipad", "IPAD_PRO_3GEN_129")],
    "TV_OS": [("tv", "APPLE_TV")],
}
summary = []
for version in versions["data"]:
    attrs = version["attributes"]
    platform = attrs["platform"]
    state = attrs["appStoreState"]
    if platform not in want:
        continue
    locs = api(f"/v1/appStoreVersions/{version['id']}/appStoreVersionLocalizations")
    en = next(loc for loc in locs["data"] if loc["attributes"].get("locale") == "en-US")
    for folder, device in want[platform]:
        if state in locked:
            line = f"skip {folder}: {platform} {attrs['versionString']} is {state}"
            print(line)
            summary.append(line)
            continue
        cmd = [
            asc, "screenshots", "upload",
            "--version-localization", en["id"],
            "--path", str(Path(out) / folder),
            "--device-type", device,
            "--replace",
        ]
        print(" ".join(cmd))
        proc = subprocess.run(cmd, text=True, capture_output=True)
        print(proc.stdout)
        print(proc.stderr, file=sys.stderr)
        if proc.returncode != 0:
            raise SystemExit(f"upload {folder} failed")
        summary.append(f"uploaded {folder} to {en['id']} ({device})")

(Path(out) / "upload.txt").write_text("\n".join(summary) + "\n")
print("\n".join(summary))
PY

validate_one() {
  local id="$1" label="$2"
  echo "validate $label $id"
  # asc exits non-zero when the report has errors. A version already in
  # review reports that it is not editable. That is not a metadata defect.
  "$ASC" validate --app "$APP_ID" --version-id "$id" --output json >"$OUT/validate-$label.json" || true
  python3 - "$OUT/validate-$label.json" "$label" <<'PY'
import json, sys
path, label = sys.argv[1], sys.argv[2]
raw = open(path).read()
start = raw.find("{")
if start < 0:
    raise SystemExit(f"{label} validate did not return JSON")
data = json.loads(raw[start:])
checks = data.get("checks") or []
locked = {"WAITING_FOR_REVIEW", "IN_REVIEW", "PENDING_DEVELOPER_RELEASE", "PENDING_APPLE_RELEASE", "READY_FOR_SALE"}
real = []
for check in checks:
    if check.get("severity") != "error":
        continue
    if check.get("id") == "version.state.editable":
        print(f"{label} {check.get('message')}")
        continue
    real.append(check)
summary = (data.get("summary") or {}).get("errors")
print(f"{label} reported errors {summary}, other errors {len(real)}")
if real:
    raise SystemExit(f"{label} errors: {real[:3]}")
PY
}

IOS_ID="$(python3 - <<'PY'
import json, os, time, urllib.request, jwt
from pathlib import Path
creds = json.loads(Path.home().joinpath(".blitz/asc-credentials.json").read_text())
now = int(time.time())
token = jwt.encode(
    {"iss": creds["issuerId"], "iat": now, "exp": now + 300, "aud": "appstoreconnect-v1"},
    creds["privateKey"], algorithm="ES256", headers={"kid": creds["keyId"], "typ": "JWT"},
)
req = urllib.request.Request(
    "https://api.appstoreconnect.apple.com/v1/apps/6815795649/appStoreVersions?limit=10",
    headers={"Authorization": f"Bearer {token}"},
)
with urllib.request.urlopen(req, timeout=60) as resp:
    data = json.load(resp)
for row in data["data"]:
    if row["attributes"]["platform"] == "IOS":
        print(row["id"])
PY
)"
TV_ID="$(python3 - <<'PY'
import json, time, urllib.request, jwt
from pathlib import Path
creds = json.loads(Path.home().joinpath(".blitz/asc-credentials.json").read_text())
now = int(time.time())
token = jwt.encode(
    {"iss": creds["issuerId"], "iat": now, "exp": now + 300, "aud": "appstoreconnect-v1"},
    creds["privateKey"], algorithm="ES256", headers={"kid": creds["keyId"], "typ": "JWT"},
)
req = urllib.request.Request(
    "https://api.appstoreconnect.apple.com/v1/apps/6815795649/appStoreVersions?limit=10",
    headers={"Authorization": f"Bearer {token}"},
)
with urllib.request.urlopen(req, timeout=60) as resp:
    data = json.load(resp)
for row in data["data"]:
    if row["attributes"]["platform"] == "TV_OS":
        print(row["id"])
PY
)"
validate_one "$IOS_ID" ios
validate_one "$TV_ID" tv
echo "screenshots are in $OUT"
