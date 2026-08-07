#!/usr/bin/env bash
set -euo pipefail

readonly public_interface="ens3"
readonly bot_ipv4="77.222.60.149/32"
readonly api_port="8000"

# Docker-published ports may bypass UFW. DOCKER-USER is the supported chain
# for policy applied before traffic reaches a container.
iptables -C DOCKER-USER -i "${public_interface}" -p tcp --dport "${api_port}" -j DROP 2>/dev/null || \
  iptables -I DOCKER-USER 1 -i "${public_interface}" -p tcp --dport "${api_port}" -j DROP
iptables -C DOCKER-USER -i "${public_interface}" -p tcp -s "${bot_ipv4}" --dport "${api_port}" -j ACCEPT 2>/dev/null || \
  iptables -I DOCKER-USER 1 -i "${public_interface}" -p tcp -s "${bot_ipv4}" --dport "${api_port}" -j ACCEPT

# У bot-хоста нет разрешённого IPv6 source address, поэтому публичный IPv6
# для API закрывается целиком. Loopback и Docker bridge не затрагиваются.
ip6tables -C DOCKER-USER -i "${public_interface}" -p tcp --dport "${api_port}" -j DROP 2>/dev/null || \
  ip6tables -I DOCKER-USER 1 -i "${public_interface}" -p tcp --dport "${api_port}" -j DROP
