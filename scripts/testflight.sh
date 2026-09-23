#!/usr/bin/env bash
# Archive the iOS and tvOS apps and upload them to TestFlight.
#
#   scripts/testflight.sh
#
# Uses the App Store Connect API key in ~/.blitz (key id + issuer in
# asc-credentials.json, private key at AuthKey_<id>.p8). The Apple ID web
# session is not required for the upload. The App Store Connect app record
# for com.wolfeup.waveguide must already exist.
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
_, bundles = call("GET", "/v1/bundleIds?filter[identifier]=com.wolfeup.waveguide&limit=5")
bundle_id = next(i["id"] for i in bundles["data"] if i["attributes"]["identifier"] == "com.wolfeup.waveguide")
wanted = {
    "IOS_APP_STORE": "Waveguide iOS App Store",
    "TVOS_APP_STORE": "Waveguide tvOS App Store",
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

EXPORT="$(mktemp -d)"
trap 'rm -rf "$EXPORT"' EXIT
archive_one() {
  local scheme="$1" platform="$2" name="$3" profile="$4"
  echo "==> archive $scheme ($platform) build $BUILD"
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
		<key>com.wolfeup.waveguide</key>
		<string>${profile}</string>
	</dict>
	<key>uploadSymbols</key>
	<true/>
</dict>
</plist>
EOF
  xcodebuild archive \
    -project Waveguide.xcodeproj \
    -scheme "$scheme" \
    -destination "generic/platform=$platform" \
    -archivePath "$APPLE/build/${name}.xcarchive" \
    DEVELOPMENT_TEAM="$TEAM" \
    CURRENT_PROJECT_VERSION="$BUILD" \
    CODE_SIGN_STYLE=Manual \
    CODE_SIGN_IDENTITY="Apple Distribution: Tyler Wolfe (D4MC63SS36)" \
    PROVISIONING_PROFILE_SPECIFIER="$profile"
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

archive_one Waveguide iOS ios "Waveguide iOS App Store"
archive_one WaveguideTV tvOS tvos "Waveguide tvOS App Store"
echo "==> uploaded build $BUILD"
