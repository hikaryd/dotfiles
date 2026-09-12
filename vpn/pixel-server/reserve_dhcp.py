#!/usr/bin/env python3
"""Reserve the current authenticated Pixel Wi-Fi address; dry-run by default."""
import argparse
import importlib.util
import ipaddress
import json
import re
import subprocess
from pathlib import Path


def phone_identity(serial):
    output = subprocess.check_output(
        ['adb', '-s', serial, 'shell', 'cmd', 'wifi', 'status'], text=True, timeout=15,
    )
    match = re.search(r'MAC: ([0-9a-fA-F:]{17}), IP: /([0-9.]+),', output)
    if not match or 'Supplicant state: COMPLETED' not in output:
        raise RuntimeError('Authenticated ADB did not return connected Wi-Fi identity')
    mac, ip = match.groups()
    ipaddress.IPv4Address(ip)
    return mac.lower(), ip


def check_reservation(mac, ip, leases, reservations, pools):
    if not isinstance(leases, list) or not isinstance(reservations, list):
        raise TypeError('Unexpected DHCP response')
    if not all(isinstance(item, dict) for item in leases + reservations):
        raise RuntimeError('Malformed DHCP entry')
    if not any(x.get('mac') == mac and x.get('ip') == ip for x in leases):
        raise RuntimeError('Router lease does not match authenticated phone identity')
    for entry in leases + reservations:
        if (entry.get('mac') == mac and entry.get('ip') != ip
                or entry.get('ip') == ip and entry.get('mac') != mac):
            raise RuntimeError('Conflicting DHCP identity/address; refusing changes')
    address = ipaddress.IPv4Address(ip)
    matching = []
    for pool in pools.values():
        bounds = pool.get('range', {})
        if (pool.get('enable') is True and bounds.get('begin') and bounds.get('end')
                and ipaddress.IPv4Address(bounds['begin']) <= address
                <= ipaddress.IPv4Address(bounds['end'])):
            matching.append(pool)
    if len(matching) != 1:
        raise RuntimeError('Address must belong to exactly one enabled DHCP pool')
    return any(x.get('mac') == mac and x.get('ip') == ip for x in reservations)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--adb-serial', required=True)
    parser.add_argument('--expected-ip', required=True)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    mac, ip = phone_identity(args.adb_serial)
    if ip != args.expected_ip:
        raise RuntimeError('Phone address differs from expected address')
    # Import helpers only. Never call router-setup.main/install (VPN writes).
    spec = importlib.util.spec_from_file_location(
        'router_setup', Path(__file__).resolve().parents[1] / 'router-setup.py',
    )
    router = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(router)
    router.auth()
    before = router.get('/rci/ip/dhcp/host')
    pools = router.get('/rci/ip/dhcp/pool')
    leases = router.get('/rci/show/ip/dhcp/bindings').get('lease')
    exists = check_reservation(mac, ip, leases, before, pools)
    saved = False
    if args.apply:
        if not exists:
            router.batch([f'ip dhcp host {mac} {ip}'])
        after = router.get('/rci/ip/dhcp/host')
        if not check_reservation(mac, ip, leases, after, pools):
            raise RuntimeError('Reservation not confirmed after write')
        if ([x for x in after if x.get('mac') != mac]
                != [x for x in before if x.get('mac') != mac]
                or router.get('/rci/ip/dhcp/pool') != pools):
            raise RuntimeError('Unrelated DHCP state changed; refusing config save')
        router.batch(['system configuration save'])
        saved = True
    print(json.dumps({'ip': ip, 'mac': mac, 'already_reserved': exists,
                      'applied': args.apply, 'configuration_save_acknowledged': saved,
                      'unrelated_reservations_preserved': True}))


if __name__ == '__main__':
    main()
