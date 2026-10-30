# Proxmox Maintainer

Projet personnel d’apprentissage et de partage, distinct du dépôt privé HOMELAB.
Lire README.md, docs/PROJECT.md et docs/ROADMAP.md avant de poursuivre.

- Répondre en français, brièvement, avec preuves et limites.
- Trois branches : main (Bash), tui-minimal (Go), advanced-tui (Go expérimental).
- Ne pas importer de secrets, IP, inventaire réel, captures ou fichiers privés du Homelab.
- Démonstrations et captures avec invités fictifs uniquement.
- Ne pas connecter/modifier des serveurs PVE sans demande explicite et impacts acceptés.
- Simulation par défaut, confirmations pour toute mutation, backup avant upgrade.
- Pas de suppression/reboot/prune/migration automatiques, pas d’édition des dépôts APT.
- Commits courts en français, dates réelles, pas d’historique fabriqué.
- Tester Bash (bash -n, scripts/test.sh) ou Go (go test ./..., go vet ./...) selon la branche.
- Vérifier les secrets avant commit/push. Ne pas publier de logs ou données personnelles.
- Maintenir la feuille de route et distinguer bêta fonctionnelle, tests simulés et fonctions futures.
