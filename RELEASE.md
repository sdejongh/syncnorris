# Guide de Release

Ce document explique comment créer une nouvelle release de syncnorris.

## Prérequis

Le processus de release est entièrement automatisé via GitHub Actions et GoReleaser. Vous n'avez besoin que de :
- Accès en écriture au repository GitHub
- Git installé localement

## Processus de Release

### 1. Préparer la release

Assurez-vous que tout est prêt :
```bash
# Vérifier que tout compile
make build

# Exécuter les tests
go test ./...

# Vérifier l'état du repository
git status
```

### 2. Mettre à jour la version dans README.md

Éditez `README.md` et mettez à jour la ligne de version :
```markdown
**Version**: 0.2.0
```

Commitez ce changement :
```bash
git add README.md
git commit -m "Bump version to 0.2.0"
git push
```

### 3. Créer et pousser un tag

```bash
# Créer un tag annoté avec la version
git tag -a v0.2.0 -m "Release v0.2.0"

# Pousser le tag vers GitHub
git push origin v0.2.0
```

### 4. GitHub Actions s'occupe du reste

Une fois le tag poussé, GitHub Actions va automatiquement :
1. ✅ Compiler le projet pour toutes les plateformes
2. ✅ Exécuter les tests
3. ✅ Créer les archives compressées
4. ✅ Générer les checksums SHA-256
5. ✅ Créer une release GitHub
6. ✅ Attacher tous les binaires à la release

Vous pouvez suivre la progression sur : `https://github.com/sdejongh/syncnorris/actions`

### 5. Vérifier la release

Une fois le workflow terminé :
1. Allez sur `https://github.com/sdejongh/syncnorris/releases`
2. Vérifiez que la release est créée
3. Vérifiez que tous les binaires sont présents
4. Téléchargez et testez un binaire

## Format de version

Utilisez le [Semantic Versioning](https://semver.org/) :
- **v0.1.0** : Release initiale
- **v0.2.0** : Ajout de nouvelles fonctionnalités
- **v0.2.1** : Correction de bugs
- **v1.0.0** : Première version stable

## Plateformes supportées

Les binaires sont compilés pour :
- **Linux** : amd64, arm64
- **macOS** : amd64, arm64 (Apple Silicon)
- **Windows** : amd64

## Annuler une release

Si vous devez annuler une release :

```bash
# Supprimer le tag localement
git tag -d v0.2.0

# Supprimer le tag sur GitHub
git push origin :refs/tags/v0.2.0
```

Puis supprimez manuellement la release sur GitHub.

## Test local de GoReleaser

Pour tester GoReleaser localement sans créer de release :

```bash
# Installer GoReleaser
go install github.com/goreleaser/goreleaser@latest

# Tester la configuration
goreleaser check

# Créer un snapshot (sans publier)
goreleaser release --snapshot --clean
```

Les binaires de test seront dans `./dist/`

## Dépannage

### Le workflow échoue avec "permission denied"
Vérifiez que le repository a les permissions nécessaires :
- Settings → Actions → General → Workflow permissions
- Sélectionnez "Read and write permissions"

### Les tests échouent
Le workflow exécute `go test ./...` avant de compiler. Assurez-vous que tous les tests passent localement.

### Version incorrecte dans le binaire
La version est injectée via ldflags. Vérifiez que `main.version` existe dans `cmd/syncnorris/main.go`.
