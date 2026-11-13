#!/bin/sh
# cage launches one app fullscreen; chromium in kiosk mode pointed at the touch UI.
exec chromium \
  --kiosk --password-store=basic  \
  --ozone-platform=wayland \
  --enable-features=UseOzonePlatform \
  --no-first-run \
  --disable-infobars \
  --disable-session-crashed-bubble \
  --disable-pinch \
  --overscroll-history-navigation=0 \
  --check-for-update-interval=31536000 \
  --app=http://localhost/propagate/ui/touch
  # Touch face (2026-11-13): the list face is the default; append '#face-plates'
  # above to boot the round-2 three-plate face instead (persists in localStorage).
