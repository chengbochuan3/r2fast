#!/usr/bin/env bash
# Build R2Fast.app — a drag-and-drop droplet around the r2fast CLI.
set -euo pipefail
cd "$(dirname "$0")/.."

APP="R2Fast.app"
rm -rf "$APP"
osacompile -o "$APP" macos/droplet.applescript

# osacompile only declares the legacy "*"/"****" wildcard document types, which
# modern macOS LaunchServices ignores — so dropped files are silently rejected.
# Declare a modern LSItemContentTypes (public.item = any file/folder) so Finder
# accepts the drop and fires the droplet's `on open` handler.
PLIST="$APP/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Add :CFBundleDocumentTypes:0:LSItemContentTypes array" "$PLIST" 2>/dev/null || true
/usr/libexec/PlistBuddy -c "Add :CFBundleDocumentTypes:0:LSItemContentTypes:0 string public.item" "$PLIST" 2>/dev/null || true
/usr/libexec/PlistBuddy -c "Set :CFBundleIdentifier com.r2fast.droplet" "$PLIST" 2>/dev/null \
  || /usr/libexec/PlistBuddy -c "Add :CFBundleIdentifier string com.r2fast.droplet" "$PLIST"

# Re-register with LaunchServices so the new document types take effect now.
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
[ -x "$LSREGISTER" ] && "$LSREGISTER" -f "$PWD/$APP" || true

echo "Built $APP — it now accepts file drops."
echo
echo "Before using it:"
echo "  - make sure 'r2fast' is on your PATH or in /usr/local/bin"
echo "  - run 'r2fast config init' once"
echo
echo "Then drag files onto R2Fast.app. Tip: move it to /Applications or your Dock."
echo "First launch: right-click -> Open to bypass Gatekeeper (unsigned app)."
