package version

// Version is the propagationd release version. Phase 0 of the v1.0 roadmap.
const Version = "0.1.0"

// Build is stamped by the Makefile (git sha + UTC minute) so a running page can
// tell when the binary under it changed and reload itself (2026-11-14).
var Build = "dev"
