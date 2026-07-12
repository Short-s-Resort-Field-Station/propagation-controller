#!/bin/sh
# cage launches one app fullscreen; chromium in kiosk mode pointed at the touch UI.
exec chromium \
  --kiosk  \
  --ozone-platform=wayland \
  --enable-features=UseOzonePlatform \
  --no-first-run \
  --disable-infobars \
  --disable-session-crashed-bubble \
  --disable-pinch \
  --overscroll-history-navigation=0 \
  --check-for-update-interval=31536000 \
  --app=http://localhost/ui/touch
