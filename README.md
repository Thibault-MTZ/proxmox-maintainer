# Proxmox Maintainer · bêta

Petit projet perso pour m’exercer sur mon homelab et simplifier les mises à jour de mes conteneurs Proxmox.

Je partage une première version **Shell**, volontairement simple. Je travaille aussi sur une interface terminal en **Go**, avec une [version minimale](https://github.com/Thibault-MTZ/proxmox-maintainer/tree/tui-minimal) et une [branche avancée expérimentale](https://github.com/Thibault-MTZ/proxmox-maintainer/tree/advanced-tui).

J’ai créé ce dépôt pour partager ce travail si ça peut être utile à d’autres et recueillir des avis. Si ça intéresse du monde, je prendrai le temps de le structurer davantage et de poursuivre les améliorations. L’historique de mon outil personnel n’est pas repris ici : le dépôt démarre avec cette première version publique.

## Version Shell

Bash, APT/dpkg et les commandes natives PVE (`pct`, `qm`, `vzdump`). Aucun Python, Go ou serveur web à installer pour cette version.

```bash
bash proxmox-update.sh --demo                  # exemples fictifs
bash proxmox-update.sh --check                 # PVE : aperçu, sans actualisation
bash proxmox-update.sh --refresh --ids 101,102  # index APT, confirmation
bash proxmox-update.sh --upgrade --ids 101,102 --backup MON-STOCKAGE
```

L’application exige une confirmation et un backup réussi. Elle concerne les **CT APT démarrés** ; hôte et VM ne sont pas mis à jour par le Shell. Pas de reboot, purge, suppression de backup ou mise à jour d’image Docker. Les services peuvent redémarrer et des données externes peuvent être exclues de la sauvegarde. À essayer dans un labo avant vos services importants.

## Installer

Sur PVE en root, depuis le dépôt :

```bash
bash install.sh --check
bash install.sh --source ./proxmox-update.sh
proxmox-update --check
```

L’installateur vérifie PVE et les dépendances. `--install-deps` autorise explicitement l’installation des paquets manquants. Il ne met aucun invité à jour. Les [releases bêta](https://github.com/Thibault-MTZ/proxmox-maintainer/releases) proposent le script et ses sommes de contrôle ; `install.sh --version TAG` installe une version publiée. L’ancienne copie installée est conservée pour retour manuel.

[Feuille de route et limites](docs/ROADMAP.md) · [Licence MIT](LICENSE)

Projet personnel, non affilié à Proxmox. Les captures des branches TUI utilisent uniquement des invités fictifs.
