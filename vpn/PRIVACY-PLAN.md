# Portable configuration cleanup

1. Lock current owner-control, DHCP, SSH and tab-cleaner behavior with existing tests.
2. Remove installation-specific addresses/identities from public sources and examples.
   Require explicit private inventory for phone identity and bind addresses; retain exact
   Host/Origin checks, pinned host keys and loopback tunnel restrictions.
3. Keep VPN installation independent of optional Pixel sources and tooling; test in a
   temporary copy containing only the VPN installer files.
4. Validate Python tests/syntax, shell syntax and a public-source privacy scan. Do not
   change deployed router, phone, VPS, keys or existing generated configurations.
