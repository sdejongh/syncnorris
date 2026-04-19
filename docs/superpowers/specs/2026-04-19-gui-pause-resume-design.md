# Pause/Resume de jobs de synchronisation (GUI)

**Date:** 2026-04-19
**Scope:** `syncnorris` GUI — ajout de la capacité de mettre en pause un job de sync et de le reprendre, y compris après une fermeture accidentelle de l'application (crash, coupure réseau, perte d'alimentation).

## 1. Contexte et motivation

`syncnorris` expose aujourd'hui deux modes de sync (one-way, bidirectionnel expérimental) pilotés par la CLI ou par la GUI Gio récemment ajoutée. La GUI propose un bouton Run/Cancel : un sync annulé est définitivement perdu, il faut tout recommencer.

Pour les transferts volumineux (plusieurs GB, plusieurs dizaines de milliers de fichiers), cette limitation devient pénible : fermer l'app pendant une nuit, un crash, ou une coupure réseau impose un redémarrage complet. L'utilisateur doit à la main relancer la sync avec les mêmes paramètres et payer le coût de revisiter toutes les comparaisons.

Cette spec définit une fonctionnalité **Pause/Resume** qui :

- Offre un bouton Pause (deux variantes : soft et hard) en plus du Cancel existant
- Persiste l'état du job pour qu'il survive à un arrêt propre, à un crash ou à une panne
- Détecte automatiquement au démarrage un job interrompu et propose à l'utilisateur de le reprendre
- Optimise la reprise pour ne pas re-traiter les fichiers déjà copiés (via un log de complétion)

## 2. Décisions clés

| Sujet | Décision | Raison |
|---|---|---|
| Granularité de reprise | Fichier (pas d'octet) | Simplicité ; acceptable en première itération |
| Périmètre | One-way uniquement | Le bidirectionnel nécessite de persister en plus la table de conflits ; à traiter séparément |
| Détection de crash | Fichier JSON + heartbeat timestamp (pas de PID) | 100% cross-platform, pas d'API OS-spécifique |
| Instance unique | Lock `github.com/gofrs/flock` | Pur Go, cross-platform, bundle statique |
| Modes de pause | Soft (laisse finir) + Hard (interrompt immédiat) | Le soft évite de perdre du travail sur gros fichiers ; le hard permet d'arrêter vite |
| Validation à la reprise | `stat(source)` + hash optionnel en filet de sécurité | Très bon marché, couvre le cas courant |
| Jobs simultanés | Un seul à la fois | Aligné avec l'UI existante (un seul bouton Run / zone progression) |
| Reprise au démarrage | Params restaurés + bannière discrète | Ni intrusif, ni muet ; l'utilisateur garde la main |

## 3. Architecture

### 3.1 Nouveau package `pkg/sync/job`

Responsable de la persistance et du cycle de vie d'un job. Isolé du pipeline pour testabilité.

```
pkg/sync/job/
  job.go         — type Job, Status, création/chargement/sauvegarde
  completion.go  — CompletionLog (append-only TSV), Write/Load/Contains
  store.go       — JobStore : list / get / delete / active, gestion du répertoire
  heartbeat.go   — goroutine qui tick toutes les 2 s pendant run
```

### 3.2 Types principaux

```go
type Status string
const (
    StatusRunning   Status = "running"
    StatusPaused    Status = "paused"
    StatusCompleted Status = "completed"
)

type Job struct {
    ID            string     // UUID
    Source        string
    Destination   string
    Options       RunOptions // mode, comparator, workers, bandwidth, exclusions, etc.
    Status        Status
    CreatedAt     time.Time
    Heartbeat     time.Time  // mis à jour pendant running
    CompletionLog string     // chemin vers le fichier de log
}

type CompletionEntry struct {
    Path  string
    Size  int64
    MTime time.Time
    Hash  string // vide si comparator non-hash
}
```

### 3.3 Emplacement des fichiers

Tout est sous `os.UserConfigDir() / syncnorris /` (cohérent avec les settings GUI existants) :

- `jobs/<id>.json` — métadonnées du job
- `jobs/<id>.log` — log de complétion (append-only TSV)
- `app.lock` — lock d'instance unique

### 3.4 Intégration au Pipeline (`pkg/sync/pipeline.go`)

- `PipelineConfig` reçoit un nouveau champ optionnel `Job *job.Job`. Si `nil`, comportement actuel (aucune persistance).
- Au démarrage, si `Job` est présent : lecture du CompletionLog et chargement en mémoire (`map[string]CompletionEntry`).
- Pendant `scanSourceAndQueue`, pour chaque fichier source :
  - Si dans le set : `stat(source)` → si `size`+`mtime` matchent → skip sans mettre en queue ; sinon fallback (re-hash si disponible, sinon flux normal)
  - Sinon → flux normal (comparator vs destination)
- Quand un worker finit un fichier avec succès : append au CompletionLog.
- Nouveaux canaux : `PauseSoft chan struct{}` et `PauseHard chan struct{}` (fermables par l'extérieur). Soft arrête le scanner ; Hard annule le context principal.

### 3.5 Écriture atomique (prérequis)

**État actuel :** `Local.Write` dans `pkg/storage/local.go` écrit directement au chemin final via `os.Create`. Un crash mid-copy laisse un fichier truncated.

**Changement :** introduire l'écriture via `<chemin>.syncnorris-<jobid[:8]>.partial` suivie d'un `rename` atomique. Ce changement bénéficie aussi au mode sans job (atomicité générale améliorée). Le suffixe intègre l'ID du job pour permettre un cleanup ciblé à l'abandon.

### 3.6 Intégration GUI (`internal/gui/`)

- **Machine à états du bouton principal** :
  - `Idle` : bouton `[Run]`
  - `Running` : boutons `[Pause Soft]` `[Pause Hard]` `[Cancel]`
  - `Paused` : boutons `[Resume]` `[Abandon]`
- Dans `appState.init()` : appel à `jobStore.Active()`. Si un job est trouvé :
  - Restauration des paramètres (source, destination, options) dans les widgets de la config panel
  - Affichage d'une bannière au-dessus de la config panel : « Un job a été interrompu — [Reprendre] [Abandonner] »
- Nouveau composant `layoutResumeBanner` dans `layout.go`.

## 4. Flux détaillés

### 4.1 Démarrage d'un nouveau job

1. Utilisateur clique Run (état `Idle`)
2. GUI appelle `jobStore.CreateJob(source, dest, options)` → génère un UUID, écrit `<id>.json` avec `status=running` et `heartbeat=now`
3. GUI démarre la goroutine heartbeat (tick 2 s → met à jour `heartbeat` dans le JSON)
4. GUI lance `Engine.Run(ctx)` avec le `*Job` injecté dans le pipeline
5. Pipeline tourne ; les workers écrivent au CompletionLog après chaque fichier réussi
6. Fin normale : `jobStore.Complete(id)` supprime `<id>.json` et `<id>.log`. UI retourne à `Idle`.

### 4.2 Pause soft

1. Utilisateur clique Pause Soft
2. GUI met `status=paused` dans le JSON, arrête le heartbeat
3. GUI ferme `PauseSoft` du pipeline
4. Le scanner arrête de pousser de nouvelles tâches
5. Les workers finissent les fichiers en cours (le `.partial` subit un rename final et une entrée est ajoutée au CompletionLog)
6. Les workers passent en attente, le pipeline reste vivant mais idle (pour permettre un Resume rapide sans recharger le CompletionLog)
7. UI affiche « Paused — N fichiers traités, M restants »

### 4.3 Pause hard

1. Utilisateur clique Pause Hard
2. GUI met `status=paused`, arrête le heartbeat
3. GUI ferme `PauseHard` → context principal annulé
4. Les workers interrompent leur copie en cours (le reader enrobé context-aware détecte `ctx.Done()` pendant l'`io.Copy`) et suppriment leur `.partial` (rollback)
5. Pipeline s'arrête proprement, un report partiel est produit
6. UI affiche « Paused — N fichiers traités, M fichiers annulés et à re-copier »

### 4.4 Reprise (Resume)

1. Utilisateur clique Resume
2. GUI appelle `jobStore.Load(id)` → met `status=running`, redémarre heartbeat
3. GUI relance `Engine.Run(ctx)` avec le même `*Job`
4. Pipeline charge le CompletionLog en mémoire (set `path → CompletionEntry`)
5. Pour chaque fichier scanné côté source :
   - Si dans le set : `stat(source)` → si `size` et `mtime` matchent → skip silencieux (incrémenté dans `skipped_from_previous_run`)
   - Si mismatch **et** hash enregistré (comparator `hash`/`md5`/`composite`) : re-hash source → si match → skip ; sinon → queue
   - Sinon → flux normal (comparator vs destination)
6. Déroulement identique à un run normal à partir de là

### 4.5 Crash et redémarrage de l'app

1. L'app crashe (segfault, kill -9, panne courant, etc.) pendant un état `running`
2. `<id>.json` reste sur disque avec `status=running` et heartbeat figé
3. Au prochain lancement : `appState.init()` → `jobStore.Active()`
4. Le store scanne `jobs/`, trouve le fichier, constate `heartbeat > 10 s` → crash détecté
5. Normalise `status=paused`, restaure les paramètres dans la GUI, affiche la bannière
6. L'utilisateur clique Reprendre (flux 4.4) ou Abandonner (flux 4.6)

### 4.6 Abandon

1. Utilisateur clique Abandon (disponible en état `Paused` ou depuis la bannière)
2. Modale de confirmation : « Les fichiers partiellement copiés seront nettoyés. Continuer ? »
3. Si confirmé : `jobStore.Abandon(id)` → parcourt la destination à la recherche de `.syncnorris-<jobid[:8]>.partial` → supprime chaque orphelin → supprime `<id>.json` et `<id>.log`
4. UI retourne à `Idle`

### 4.7 Instance unique

- Au démarrage, `flock` non-bloquant sur `app.lock`
- Si le lock est déjà pris → message d'erreur « syncnorris est déjà en cours d'exécution » et exit code 1
- Lock relâché automatiquement au shutdown propre ou à la mort du process

### 4.8 Cas limites

- **Démarrer un nouveau job alors qu'un job en pause existe :** au clic sur Run, si `jobStore.Active()` retourne un job, prompt « Abandonner le job en pause et démarrer un nouveau ? » (pas de fusion automatique)
- **Fichier `.partial` orphelin côté destination :** le suffixe unique `<name>.syncnorris-<jobid[:8]>.partial` permet un cleanup ciblé à l'abandon sans toucher aux fichiers d'un autre outil ou d'un autre job
- **Changement de source ou destination entre sessions :** non supporté. L'utilisateur doit abandonner pour changer de chemins (les chemins sont figés dans le fichier de job).

## 5. Formats de données

### 5.1 Fichier de job `<id>.json`

```json
{
  "id": "a3f9b2e1-4c7d-48a0-9b22-1e0b4c9f5a8d",
  "source": "/home/user/data",
  "destination": "/mnt/backup/data",
  "options": {
    "mode": "one-way",
    "comparator": "hash",
    "workers": 5,
    "bandwidth_limit": 0,
    "delete_orphans": false,
    "exclude_patterns": [".git", "*.tmp"]
  },
  "status": "running",
  "created_at": "2026-04-19T10:15:00Z",
  "heartbeat": "2026-04-19T10:22:34Z",
  "completion_log": "/home/user/.config/syncnorris/jobs/a3f9b2e1-....log"
}
```

Écrit atomiquement via `write-to-temp + rename`.

### 5.2 Log de complétion `<id>.log`

Format TSV append-only, une ligne par fichier complété :

```
<path>\t<size>\t<mtime_unix_nanos>\t<hash_or_dash>\n
```

Exemple :

```
docs/readme.md	1248	1742034567000000000	e3b0c44298fc1c149afbf4c8996fb924...
images/logo.png	87234	1741998123000000000	-
```

- Écritures groupées par batch : flush toutes les 100 entrées OU toutes les 500 ms (le premier événement qui se présente)
- `fsync` au flush pour garantir la durabilité face au crash
- Chargement d'un resume : lecture complète → `map[string]CompletionEntry`, O(1) lookup

### 5.3 Suffixe des fichiers partiels

`<path>.syncnorris-<jobid[:8]>.partial`

Pour les runs sans job (CLI standard, backward compat), le suffixe est `<path>.syncnorris.partial` (pas d'ID).

### 5.4 Lock d'instance

`app.lock` — fichier vide, lock exclusif non-bloquant via `gofrs/flock`. Aucun contenu (le lock OS suffit).

## 6. Gestion des erreurs

| Situation | Comportement |
|---|---|
| Écriture du JSON de job échoue | Le job démarre quand même, warning logué. Sans fichier, pas de recovery possible — non bloquant. |
| Écriture au CompletionLog échoue | Warning logué, on continue. Perte de l'entrée = ce fichier sera retraité à la reprise (le comparator le skippera si identique). Non critique. |
| Fichier de job corrompu au chargement | Erreur loguée, on ignore le job (pas de bannière). Commande CLI `syncnorris jobs clear` en option future. |
| CompletionLog partiellement corrompu | Parser tolérant : lignes invalides ignorées. Pire cas : quelques fichiers retraités inutilement. |
| Lock d'instance impossible à prendre | Erreur claire, exit code 1. |
| Heartbeat échoue à écrire | Warning, on continue. Catch-up au prochain tick réussi. |
| Source ou destination indisponible à la reprise | Comportement standard du pipeline (erreur et arrêt). L'utilisateur peut abandonner ou réessayer. |
| `.partial` impossible à supprimer au hard-pause | Erreur loguée, le fichier sera écrasé à la reprise. |

## 7. Tests

### 7.1 Unitaires (`pkg/sync/job/`)

- `Job` sérialisation (roundtrip JSON, champs `Options`)
- `CompletionLog` append + load, incluant batch flush et lignes malformées
- `JobStore` : create / list / get / delete / active, détection de crash via heartbeat stale
- Heartbeat : tick régulier + arrêt propre

### 7.2 Intégration (`pkg/sync/`)

- Pause soft sur pipeline en cours : compte correct de fichiers traités, aucun `.partial` résiduel
- Pause hard : context annulé, `.partial` nettoyés
- Resume après pause : fichiers « done » skippés, restants traités
- Resume après modification de source entre-temps : fichier modifié re-copié
- Simulation de crash (arrêt goroutine + heartbeat stale injecté) puis resume
- Écriture atomique : vérifier qu'aucun fichier partiel ne subsiste au chemin final après interruption

### 7.3 GUI (`internal/gui/`)

- Machine à états du bouton principal (transitions Idle → Running → Paused → Running → Idle)
- Restauration des paramètres depuis un job interrompu au startup
- Affichage correct de la bannière et des actions associées

### 7.4 Observabilité

- Tous les événements de transition (`job.Started`, `job.Paused`, `job.Resumed`, `job.Completed`, `job.Abandoned`) logués via le logger existant
- Champ `job_id` attaché à tous les logs pendant un run (propagé via context ou champ structuré)

## 8. Migration et scope

### 8.1 Rétro-compatibilité

- Les formats `SyncOperation` et `SyncReport` existants ne changent pas
- La CLI continue de fonctionner sans job : pas de persistance activée si l'appelant ne fournit pas de `*Job`
- Les settings GUI existants (`gui-settings.json`) ne sont pas touchés ; nouveau sous-répertoire `jobs/`
- Changement d'écriture atomique via `.partial` : amélioration neutre pour la CLI (pas de régression)

### 8.2 Surface publique du code

- `NewPipeline` / `PipelineConfig` : nouveau champ optionnel `Job *job.Job`
- Nouveaux canaux optionnels dans `PipelineConfig` : `PauseSoft`, `PauseHard`
- `Local.Write` : passe à un schéma write-then-rename (comportement externe inchangé pour les consommateurs)

### 8.3 Versionnage

Bump **minor** : `v0.8.0`. Pas de breaking change CLI.

### 8.4 Documentation à mettre à jour

- `CLAUDE.md` : section GUI, mention du job store, du lock d'instance et du schéma d'écriture atomique
- `README.md` : nouvelle section « Pause/Resume » avec captures
- `CHANGELOG.md` : entrée `v0.8.0`

### 8.5 Scope volontairement exclu

- Sync bidirectionnelle (reste non-pausable)
- Reprise au niveau octet (granularité fichier seulement)
- Plusieurs jobs en parallèle dans la même instance
- Reprise sur une machine différente (jobs liés au `os.UserConfigDir()` local)
- Notifications système lors d'événements de job
