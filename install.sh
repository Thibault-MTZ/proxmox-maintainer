#!/usr/bin/env bash
# Installs only the small Shell edition; does not update any guest.
set -Eeuo pipefail
umask 077
version=v0.1.0-beta.1 source= deps=0 check=0
die(){ echo "ERREUR : $*" >&2; exit 1; }
while (($#)); do
 case "$1" in
  --source|--version) (($#>=2)) || die 'valeur manquante'; if [[ $1 == --source ]]; then source=$2; else version=$2; fi; shift;;
  --install-deps) deps=1;; --check) check=1;;
  -h|--help) echo 'install.sh [--check] [--source SCRIPT | --version TAG] [--install-deps]'; exit 0;;
  *) die "argument inconnu $1";;
 esac; shift
done
[[ $EUID == 0 && -d /etc/pve ]] || die 'root sur Proxmox VE requis ; ne pas installer PVE avec ce script'
command -v pct >/dev/null || die 'pct absent : installation PVE non reconnue'
missing=()
for tool in flock curl sha256sum awk cmp; do command -v "$tool" >/dev/null || missing+=("$tool"); done
if ((${#missing[@]})); then
 printf 'Dépendances absentes : %s\n' "${missing[*]}"
 ((check==0)) || exit 1
 ((deps==1)) || die 'relancer avec --install-deps pour autoriser APT (curl, ca-certificates, util-linux, coreutils, gawk, diffutils)'
 command -v apt-get >/dev/null || die 'APT indisponible'
 apt-get update
 apt-get install -y --no-install-recommends curl ca-certificates util-linux coreutils gawk diffutils
fi
((check==0)) || { echo 'PVE et dépendances détectés ; aucune installation'; exit 0; }
tmp=$(mktemp -d)
trap 'rm -f -- "$tmp"/proxmox-update.sh "$tmp"/SHA256SUMS; rmdir -- "$tmp"' EXIT
if [[ -n $source ]]; then cp -- "$source" "$tmp/proxmox-update.sh"
else
 [[ $version =~ ^v[0-9][a-zA-Z0-9._-]*$ ]] || die 'tag invalide'
 url="https://github.com/Thibault-MTZ/proxmox-maintainer/releases/download/$version"
 curl --fail --location --proto '=https' --tlsv1.2 "$url/proxmox-update.sh" -o "$tmp/proxmox-update.sh"
 curl --fail --location --proto '=https' --tlsv1.2 "$url/SHA256SUMS" -o "$tmp/SHA256SUMS"
 (cd "$tmp" && grep -E '^[a-f0-9]{64}  proxmox-update.sh$' SHA256SUMS | sha256sum -c -) || die 'somme de contrôle invalide'
fi
bash -n "$tmp/proxmox-update.sh"
target=/usr/local/sbin/proxmox-update
if [[ -e $target ]]; then
 grep -q 'Simple CT APT maintenance' "$target" || die 'commande étrangère existante'
 mkdir -p /var/lib/proxmox-maintainer
 cp -p -- "$target" "/var/lib/proxmox-maintainer/proxmox-update.before-$(date +%s)"
fi
install -m 755 "$tmp/proxmox-update.sh" "$target.new"
mv -f -- "$target.new" "$target"
echo 'Installé : proxmox-update --check. Aucun invité modifié.'
