#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
bash -n proxmox-update.sh install.sh
bash proxmox-update.sh --demo --check | grep -q 'lab-web'
if bash proxmox-update.sh --demo --upgrade --backup TEST >/dev/null 2>&1; then echo 'upgrade demo accepté' >&2; exit 1; fi
if bash proxmox-update.sh --ids '101;bad' >/dev/null 2>&1; then echo 'IDs invalides acceptés' >&2; exit 1; fi
if bash install.sh --unknown >/dev/null 2>&1; then echo 'argument install accepté' >&2; exit 1; fi
echo 'Tests Shell OK (simulation ; aucun PVE modifié)'
