#!/bin/sh
set -eu

for tool in iptables ip6tables; do
    for chain in INPUT OUTPUT FORWARD; do
        "$tool" -P "$chain" DROP
    done
done

[ "$#" -eq 3 ] || exit 2
ip=$1
port=$2
gateway=$3
case "$gateway" in *[!0-9.]*|'') exit 2;; esac
case "$ip" in *[!0-9.]*|'') exit 2;; esac
case "$port" in *[!0-9]*|'') exit 2;; esac
[ "$port" -ge 1 ] && [ "$port" -le 65535 ] || exit 2
old_ifs=$IFS
IFS=.
set -- $ip
IFS=$old_ifs
[ "$#" -eq 4 ] || exit 2
for octet do
    case "$octet" in ''|*[!0-9]*|0[0-9]*) exit 2;; esac
    [ "$octet" -le 255 ] || exit 2
done
[ "$1" -ne 127 ] && [ "$1" -ne 0 ] || exit 2

iptables -A INPUT -s "$ip" -p tcp --sport "$port" -m conntrack --ctstate ESTABLISHED -j ACCEPT
iptables -A INPUT -s "$gateway" -p tcp --dport 5900 -m conntrack --ctstate NEW,ESTABLISHED -j ACCEPT
iptables -A OUTPUT -d "$ip" -p tcp --dport "$port" -j ACCEPT
iptables -A OUTPUT -d "$gateway" -p tcp --sport 5900 -m conntrack --ctstate ESTABLISHED -j ACCEPT
printf 'FIREWALL_READY\n'
exec sleep infinity
