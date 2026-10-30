# État de la bêta — 30 octobre 2026

## Trois pistes

- **main / Shell** : aperçu APT des CT actifs, actualisation explicite, backup obligatoire avant application, contrôles d’identité et de dérive, journal privé. VM visibles seulement.
- **tui-minimal / Go** : inventaire CT/VM, sélection, versions candidates, confirmation, backup et application APT. VM Linux via alias SSH configuré explicitement depuis PVE. Démonstration sans connexion serveur.
- **advanced-tui / Go** : même base, découverte expérimentale des applications/services/ports et Docker ; chantier destiné à recueillir des avis.

## Prochaines étapes

1. Retours sur le parcours et essais dans des CT/VM de labo dédiés.
2. Améliorer progression, erreurs et navigation sur petits terminaux.
3. Tester coupure SSH, backup échoué, dépôts indisponibles et reprise d’une tâche réelle.
4. Releases de binaires Go, installation automatique du TUI puis proposition de mise à jour depuis une release vérifiée, avec retour à la version précédente.
5. Adaptateurs applicatifs et Docker Compose, puis dépôts/canaux selon les fournisseurs.

## Limites à connaître

- Tests Go avec commandes simulées : pas de preuve d’une mise à jour/restauration d’une VM réelle.
- Alias SSH VM à définir manuellement ; pas de découverte d’IP ni de configuration QEMU Guest Agent.
- Backup snapshot VM potentiellement crash-consistent. Données externes/bind mounts et bases déportées pas forcément couvertes ; pas de rollback APT transactionnel.
- APT peut redémarrer des services, y compris DNS/VPN/proxy. Aucun reboot de l’hôte ou des invités automatiquement.
- L’hôte PVE, Alpine, les images Docker, les dépôts/canaux et les snapshots ne sont pas mis à jour par cette bêta.
- Le verrou de l’outil n’exclut pas les autres outils d’administration. Après interruption système/SSH, vérifier journaux et dpkg avant de relancer ; pas de retrait forcé de verrou.
- Aucun bug volontaire. Les problèmes constatés et les retours seront suivis dans les issues ; ces limites ne sont pas des fonctions annoncées comme terminées.
