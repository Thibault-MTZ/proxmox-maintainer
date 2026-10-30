#!/usr/bin/env bash
# Simple CT APT maintenance. Run on PVE; defaults to simulation.
set -Eeuo pipefail
umask 077
mode=check ids= storage= demo=0
die(){ printf 'ERREUR : %s\n' "$*" >&2; exit 1; }
while (($#)); do
 case "$1" in
  --check) mode=check;; --refresh) mode=refresh;; --upgrade) mode=upgrade;;
  --ids|--backup) (($#>=2)) || die "valeur requise pour $1"; if [[ $1 == --ids ]]; then ids=$2; else storage=$2; fi; shift;;
  --demo) demo=1;;
  -h|--help) echo 'proxmox-update.sh [--demo] [--check|--refresh|--upgrade] [--ids 101,102] [--backup STOCKAGE]'; exit 0;;
  *) die "argument inconnu : $1";;
 esac; shift
done
[[ -z $storage || $storage =~ ^[a-zA-Z][a-zA-Z0-9_.-]*$ ]] || die 'stockage invalide'
[[ -z $ids || $ids =~ ^[1-9][0-9]*(,[1-9][0-9]*)*$ ]] || die 'IDs invalides'
if ((demo)); then
 [[ $mode == check ]] || die 'démo : aucune mutation autorisée'
 printf 'DÉMO — invités fictifs\nCT101 lab-web : actif, curl 8.1 → 8.2\nCT102 lab-docker : actif, aucune mise à jour\nCT104 lab-offline : arrêté, ignoré\nVM201 debian-demo : visible, maintenance réservée au TUI\n'
 exit 0
fi
[[ $EUID == 0 && -d /etc/pve ]] || die 'root sur un hôte Proxmox VE requis'
for tool in pct flock awk sha256sum install mktemp; do command -v "$tool" >/dev/null || die "dépendance manquante : $tool (voir install.sh)"; done
[[ $mode != upgrade || -n $storage ]] || die '--upgrade exige --backup STOCKAGE'
[[ $mode != upgrade ]] || command -v vzdump >/dev/null || die 'vzdump absent'
exec 9>/run/lock/proxmox-update.lock
flock -n 9 || die 'une autre exécution est active'
work=$(mktemp -d /run/proxmox-update.XXXXXX)
trap 'rm -f -- "$work"/*.plan; rmdir -- "$work"' EXIT
identity(){
 local config machine
 config=$(pct config "$1") || return 1
 [[ $config != *$'\nlock:'* && $config != lock:* && $config != *'template: 1'* ]] || return 1
 machine=$(pct exec "$1" -- cat /etc/machine-id) || return 1
 [[ $machine =~ ^[a-f0-9]{32}$ ]] || return 1
 printf '%s\n%s\n' "$config" "$machine" | sha256sum | awk '{print $1}'
}
simulate(){ pct exec "$1" -- env LC_ALL=C apt-get -s --no-remove upgrade; }
listing=$(pct list) || die 'inventaire CT indisponible'
mapfile -t all < <(printf '%s\n' "$listing" | awk 'NR>1 {print $1}')
if [[ -n $ids ]]; then IFS=, read -ra selected <<<"$ids"; else selected=("${all[@]}"); fi
declare -a eligible=()
declare -A identities=()
errors=0
printf 'PROXMOX MAINTAINER — %s\n' "$mode"
if command -v qm >/dev/null; then qm list | awk 'NR>1 {printf "VM%s %s : visible, non gérée par Shell\n",$1,$2}'; fi
for id in "${selected[@]}"; do
 [[ " ${all[*]} " == *" $id "* ]] || die "CT$id absent"
 status=$(pct status "$id") || die "état CT$id indisponible"
 [[ $status == *running* ]] || { printf 'CT%s : arrêté, ignoré\n' "$id"; continue; }
 if ! fingerprint=$(identity "$id"); then printf 'CT%s : identité non vérifiable, verrou ou modèle — ignoré\n' "$id"; continue; fi
 if ! pct exec "$id" -- test -x /usr/bin/apt-get; then printf 'CT%s : APT absent, ignoré\n' "$id"; continue; fi
 if ! simulate "$id" >"$work/$id.plan"; then printf 'CT%s : simulation échouée\n' "$id"; errors=1; continue; fi
 if grep -Eq '^(Remv|Purg) ' "$work/$id.plan"; then die "CT$id : suppressions refusées"; fi
 printf '\nCT%s — versions candidates (cache actuel)\n' "$id"
 awk '/^Inst / {print "  " $0; found=1} END {if (!found) print "  Aucun paquet à mettre à jour"}' "$work/$id.plan"
 eligible+=("$id"); identities[$id]=$fingerprint
done
[[ $mode != check ]] || exit "$errors"
((errors==0)) || die 'corriger les erreurs avant une mutation'
((${#eligible[@]})) || { echo 'Aucun CT admissible'; exit 0; }
phrase=ACTUALISER
if [[ $mode == upgrade ]]; then
 phrase=METTRE-A-JOUR
 printf '\nSauvegarde : %s. Services susceptibles de redémarrer. Backups conservés.\nLes bind mounts et bases externes peuvent être exclus ; restauration non testée.\n' "$storage"
fi
printf 'Cibles : %s\nTaper %s : ' "${eligible[*]}" "$phrase"
[[ -t 0 ]] || die 'confirmation interactive requise'
read -r answer; [[ $answer == "$phrase" ]] || die 'annulé'
log=/var/log/proxmox-update-$(date +%Y%m%d-%H%M%S).log
install -m 600 /dev/null "$log"
for id in "${eligible[@]}"; do
 [[ $(identity "$id") == "${identities[$id]}" ]] || die "CT$id modifié : recréer le plan"
 if [[ $mode == refresh ]]; then
  pct exec "$id" -- env LC_ALL=C apt-get update >>"$log" 2>&1 || die "actualisation échouée CT$id ; $log"
  continue
 fi
 grep -q '^Inst ' "$work/$id.plan" || continue
 pct exec "$id" -- dpkg --audit >"$work/audit.plan" || die 'audit dpkg en échec'
 [[ ! -s $work/audit.plan ]] || die 'dpkg incohérent'
 pct exec "$id" -- apt-get check >>"$log" 2>&1 || die 'dépendances incohérentes'
 free=$(pct exec "$id" -- df -Pk / | awk 'END {print $4}')
 [[ $free =~ ^[0-9]+$ ]] && ((free>=524288)) || die "CT$id : moins de 512Mio libres"
 printf 'CT%s : sauvegarde puis application…\n' "$id"
 vzdump "$id" --storage "$storage" --mode snapshot --remove 0 >>"$log" 2>&1 || die "backup CT$id échoué ; application bloquée"
 [[ $(identity "$id") == "${identities[$id]}" ]] || die "identité CT$id modifiée"
 simulate "$id" >"$work/again.plan" || die 'revalidation APT échouée'
 cmp -s "$work/$id.plan" "$work/again.plan" || die 'plan périmé ; sauvegarde conservée'
 pct exec "$id" -- env DEBIAN_FRONTEND=noninteractive LC_ALL=C apt-get -y --no-remove -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold upgrade >>"$log" 2>&1 || die "upgrade CT$id échoué ; $log"
 pct exec "$id" -- dpkg --audit >"$work/audit.plan" || die 'audit final en échec'
 [[ ! -s $work/audit.plan ]] || die 'dpkg final à examiner'
 pct exec "$id" -- apt-get check >>"$log" 2>&1 || die 'dépendances finales à examiner'
 printf 'CT%s : APT terminé. Vérifier les applications.\n' "$id"
done
printf 'Terminé ; journal privé : %s. Aucun reboot ni nettoyage automatique.\n' "$log"
