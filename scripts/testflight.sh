#!/usr/bin/env bash
# Archive the iOS and tvOS apps and upload them to TestFlight.
#
#   scripts/testflight.sh
#
# Uses the App Store Connect API key in ~/.blitz (key id + issuer in
# asc-credentials.json, private key at AuthKey_<id>.p8). The Apple ID web
# session is not required for the upload. The App Store Connect app record
# for com.wolfeup.broadwave must already exist.
#
# Build number is the number of commits on this checkout. Team is
# D4MC63SS36 (Tyler Wolfe), the same team as the other com.wolfeup apps.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APPLE="$ROOT/apple"
BUILD="$(git -C "$ROOT" rev-list --count HEAD)"
KEY_JSON="${ASC_KEY_JSON:-$HOME/.blitz/asc-credentials.json}"
KEY_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["keyId"])' "$KEY_JSON")"
ISSUER="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["issuerId"])' "$KEY_JSON")"
KEY_PATH="${ASC_KEY_PATH:-$HOME/.blitz/AuthKey_${KEY_ID}.p8}"
TEAM="${DEVELOPMENT_TEAM:-D4MC63SS36}"
test -f "$KEY_PATH"

# Xcode on this machine has no Apple ID account, so automatic signing cannot
# create profiles. The API key can. App Store profiles do not need devices.
python3 - "$KEY_JSON" "$HOME/Library/MobileDevice/Provisioning Profiles" << 'PY'
import base64, json, sys, urllib.request, urllib.error, time
from pathlib import Path
import jwt
key_json, dest = sys.argv[1], Path(sys.argv[2])
dest.mkdir(parents=True, exist_ok=True)
creds = json.loads(Path(key_json).read_text())
now = int(time.time())
token = jwt.encode(
    {"iss": creds["issuerId"], "iat": now, "exp": now + 600, "aud": "appstoreconnect-v1"},
    creds["privateKey"], algorithm="ES256",
    headers={"kid": creds["keyId"], "typ": "JWT"},
)
base = "https://api.appstoreconnect.apple.com"

def call(method, path, payload=None):
    data = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(base + path, data=data, method=method, headers={
        "Authorization": f"Bearer {token}", "Content-Type": "application/json",
    })
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read()
            return resp.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        raise SystemExit(f"profiles {method} {path} -> {e.code} {e.read()[:400].decode()}")

_, certs = call("GET", "/v1/certificates?filter[certificateType]=DISTRIBUTION&limit=5")
cert_id = certs["data"][0]["id"]
_, bundles = call("GET", "/v1/bundleIds?filter[identifier]=com.wolfeup.broadwave&limit=5")
bundle_id = next(i["id"] for i in bundles["data"] if i["attributes"]["identifier"] == "com.wolfeup.broadwave")
wanted = {
    "IOS_APP_STORE": "Broadwave iOS App Store",
    "TVOS_APP_STORE": "Broadwave tvOS App Store",
}
_, existing = call("GET", "/v1/profiles?limit=200")
have = {i["attributes"]["profileType"]: i for i in existing.get("data", []) if i["attributes"]["name"] in wanted.values()}
for ptype, name in wanted.items():
    item = have.get(ptype)
    if item is None:
        _, created = call("POST", "/v1/profiles", {"data": {
            "type": "profiles",
            "attributes": {"name": name, "profileType": ptype},
            "relationships": {
                "bundleId": {"data": {"type": "bundleIds", "id": bundle_id}},
                "certificates": {"data": [{"type": "certificates", "id": cert_id}]},
            },
        }})
        item = created["data"]
        print(f"created profile {name}")
    content = item["attributes"].get("profileContent")
    if not content:
        _, full = call("GET", f"/v1/profiles/{item['id']}")
        content = full["data"]["attributes"]["profileContent"]
    uuid = item["attributes"]["uuid"]
    (dest / f"{uuid}.mobileprovision").write_bytes(base64.b64decode(content))
    print(f"installed {name} ({uuid})")
PY

cd "$APPLE"
xcodegen generate

# Command-line PROVISIONING_PROFILE_SPECIFIER reaches Swift packages, and
# Xcode 27 refuses to archive them. Write the profile onto the app targets only.
python3 - "$APPLE/Broadwave.xcodeproj/project.pbxproj" << 'PY'
import re, sys
path = sys.argv[1]
text = open(path).read()
identity = "Apple Distribution: Tyler Wolfe (D4MC63SS36)"
profiles = {
    "iphoneos": "Broadwave iOS App Store",
    "appletvos": "Broadwave tvOS App Store",
}

patched = 0

def patch(m):
    global patched
    body = m.group(0)
    if "INFOPLIST_FILE" not in body or "CODE_SIGNING_ALLOWED = NO" in body:
        return body
    sdk = re.search(r"SDKROOT = (\w+);", body)
    bundle = re.search(r"PRODUCT_BUNDLE_IDENTIFIER = ([^;]+);", body)
    if not sdk or not bundle or bundle.group(1).strip() != "com.wolfeup.broadwave":
        return body
    profile = profiles.get(sdk.group(1))
    if not profile:
        return body

    def set_key(body, key, value):
        line = f"\t\t\t\t{key} = {value};\n"
        found = re.search(rf"\t*{key} = [^;]*;\n", body)
        if found:
            return body[:found.start()] + line + body[found.end():]
        return body.replace("buildSettings = {\n", "buildSettings = {\n" + line, 1)

    body = set_key(body, "CODE_SIGN_IDENTITY", '"%s"' % identity)
    body = set_key(body, "CODE_SIGN_STYLE", "Manual")
    body = set_key(body, "PROVISIONING_PROFILE_SPECIFIER", '"%s"' % profile)
    patched += 1
    return body

updated = re.sub(
    r"[A-F0-9]{24} /\* (?:Debug|Release) \*/ = \{\n\t\t\tisa = XCBuildConfiguration;\n\t\t\tbuildSettings = \{.*?\n\t\t\t\};",
    patch,
    text,
    flags=re.S,
)
if patched != 4:
    raise SystemExit(f"expected 4 app configurations, patched {patched}")
open(path, "w").write(updated)
print("provisioning profiles set on the app targets")
PY

EXPORT="$(mktemp -d)"
trap 'rm -rf "$EXPORT"' EXIT
archive_one() {
  local scheme="$1" platform="$2" name="$3" profile="$4"
  echo "==> archive $scheme ($platform) build $BUILD ($profile)"
  cat > "$EXPORT/ExportOptions.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>method</key>
	<string>app-store-connect</string>
	<key>destination</key>
	<string>upload</string>
	<key>teamID</key>
	<string>${TEAM}</string>
	<key>signingStyle</key>
	<string>manual</string>
	<key>signingCertificate</key>
	<string>Apple Distribution</string>
	<key>provisioningProfiles</key>
	<dict>
		<key>com.wolfeup.broadwave</key>
		<string>${profile}</string>
	</dict>
	<key>uploadSymbols</key>
	<true/>
</dict>
</plist>
EOF
  # The profile lives on the app target only. A command-line override would
  # also land on BroadwaveKit, which Xcode 27 will not sign that way.
  xcodebuild archive \
    -project Broadwave.xcodeproj \
    -scheme "$scheme" \
    -destination "generic/platform=$platform" \
    -archivePath "$APPLE/build/${name}.xcarchive" \
    DEVELOPMENT_TEAM="$TEAM" \
    CURRENT_PROJECT_VERSION="$BUILD"
  if [[ "${SKIP_UPLOAD:-}" == "1" ]]; then
    echo "==> skipped upload (SKIP_UPLOAD=1)"
    return 0
  fi
  echo "==> upload $scheme"
  xcodebuild -exportArchive \
    -archivePath "$APPLE/build/${name}.xcarchive" \
    -exportPath "$APPLE/build/export-${name}" \
    -exportOptionsPlist "$EXPORT/ExportOptions.plist" \
    -authenticationKeyPath "$KEY_PATH" \
    -authenticationKeyID "$KEY_ID" \
    -authenticationKeyIssuerID "$ISSUER"
}

archive_one Broadwave iOS ios "Broadwave iOS App Store"
archive_one BroadwaveTV tvOS tvos "Broadwave tvOS App Store"
echo "==> uploaded build $BUILD"
