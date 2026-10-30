# Reprise du projet

## Intention

Thibault souhaite apprendre en construisant un petit outil utile pour son homelab, le partager et recueillir des avis avant d’investir dans une application plus complète. Le dépôt public commence avec une bêta ; aucun ancien historique n’est importé.

## Organisation

GitHub : https://github.com/Thibault-MTZ/proxmox-maintainer

Local Windows :

- `proxmox-maintainer` — main/Shell.
- `../proxmox-maintainer-tui` — tui-minimal.
- `../proxmox-maintainer-advanced` — advanced-tui.

Les deux dossiers TUI sont des worktrees du même dépôt, reliés au même remote origin. Le prototype Python et l’ancien historique restent archivés séparément ; ne pas les republier ni réécrire le dépôt sans nouvelle demande.

## Direction future

Interface Go avec Bubble Tea, Bubbles et Lip Gloss, commandes Cobra, configuration JSON. Binaire Linux autonome pour éviter d’installer une toolchain sur PVE. Même moteur pour modes CLI/TUI, catalogue d’applications et règles explicites ; pas d’exécution de scripts arbitraires issus d’un catalogue.

À terme : inventaire dynamique, plans complets, protection avant mises à jour, contrôles après application, applications et Docker selon adaptateurs, dépôts/canaux séparés. Les mises à jour du programme passent par des releases identifiées, vérifiées et confirmées, pas par un git pull automatique en root.

## Périmètre actuel

Voir ROADMAP.md pour les limites. Aucun CT/VM de production ne doit servir aux captures. Les tests unitaires ne valent pas validation d’une mise à jour, restauration ou UX réelle. Aucune infrastructure n’est gérée automatiquement par ce projet.
