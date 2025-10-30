# Flutter Build System - Documentation

## Vue d'ensemble

Ce système permet de builder des applications Flutter (Android, iOS, Web) dans des pods Kubernetes isolés avec gestion complète des variables d'environnement, fichiers de configuration et keystores de signature.

## Architecture

### Composants

1. **Dockerfile** (`build/flutter-build.Dockerfile`)
   - Image de base Ubuntu 22.04
   - Android SDK complet (build-tools, platforms, NDK)
   - Java 17
   - Flutter (canal configurable)
   - Script de build intégré

2. **Script de Build** (`build/build.sh`)
   - Clone du dépôt Git
   - Configuration du canal Flutter
   - Gestion des fichiers d'environnement
   - Configuration du keystore Android
   - Build de l'application
   - Copie des artifacts

3. **Module Kubernetes** (`pkg/kubernetes/`)
   - `pod.go` : Création et gestion des pods de build
   - `resources.go` : Gestion des ConfigMaps, Secrets et PVC

4. **Modèles de données** (`pkg/db/models.go`)
   - `Env` : Variables d'environnement et fichiers
   - `Keystore` : Credentials de signature Android

## Construction de l'image Docker

```bash
cd build/
docker build -f flutter-build.Dockerfile -t flotio/flutter-build:latest .
docker push flotio/flutter-build:latest
```

## Configuration

### Variables d'environnement du système

- `K8S_NAMESPACE` : Namespace Kubernetes (défaut: "default")
- `FLUTTER_BUILD_IMAGE` : Image Docker à utiliser (défaut: "flotio/flutter-build:latest")
- `KUBECTL_API` : URL de l'API Kubernetes (si hors cluster)
- `KUBECTL_TOKEN` : Token d'authentification (si hors cluster)

## Utilisation

### 1. Configuration d'un projet

#### Ajouter des variables d'environnement

```go
env := db.Env{
    ProjectID: projectID,
    Key:       "API_KEY",
    Value:     "your-api-key",
    Type:      "env",
}
db.DB.Create(&env)
```

#### Ajouter des fichiers de configuration

```go
// Exemple : google-services.json pour Android
googleServicesContent := `{"project_info": {...}}`
env := db.Env{
    ProjectID: projectID,
    Key:       "google-services",
    Value:     googleServicesContent,
    Type:      "file",
    Path:      "android/app/google-services.json",
    IsBase64:  false,
}
db.DB.Create(&env)

// Exemple : fichier binaire (base64)
keystoreBytes, _ := ioutil.ReadFile("release.keystore")
keystoreB64 := base64.StdEncoding.EncodeToString(keystoreBytes)
env := db.Env{
    ProjectID: projectID,
    Key:       "release-keystore",
    Value:     keystoreB64,
    Type:      "file",
    Path:      "android/app/release.keystore",
    IsBase64:  true,
}
db.DB.Create(&env)
```

#### Configurer un keystore Android

```go
keystoreFile, _ := ioutil.ReadFile("my-release-key.jks")
keystoreB64 := base64.StdEncoding.EncodeToString(keystoreFile)

keystore := db.Keystore{
    ProjectID:     projectID,
    Name:          "Production Keystore",
    KeystoreFile:  keystoreB64,
    StorePassword: "store-password",
    KeyAlias:      "key-alias",
    KeyPassword:   "key-password",
    IsActive:      true,
}
db.DB.Create(&keystore)
```

### 2. Lancer un build

```go
config := kubernetes.BuildConfig{
    BuildID:        buildID,
    Project:        project,
    Platform:       "android",
    BuildMode:      "release",
    BuildTarget:    "apk", // ou "aab" pour App Bundle
    FlutterChannel: "stable",
    GitBranch:      "main",
    GitUsername:    "", // Optionnel
    GitPassword:    "", // Optionnel
}

err := kubernetes.CreateBuildPod(config)
if err != nil {
    log.Printf("Failed to create build pod: %v", err)
}
```

### 3. Suivre le build

```go
// Récupérer le statut
status, err := kubernetes.GetPodStatus(buildID)
fmt.Printf("Build status: %s\n", status)

// Streamer les logs en temps réel
logChan := make(chan string)
go kubernetes.StreamPodLogs(buildID, logChan)

for log := range logChan {
    fmt.Print(log)
}
```

### 4. Nettoyer les ressources

```go
config, _ := kubernetes.getKubernetesConfig()
clientset, _ := kubernetes.NewForConfig(config)
namespace := kubernetes.getNamespace()

err := kubernetes.DeleteBuildResources(clientset, buildID, namespace)
if err != nil {
    log.Printf("Failed to cleanup: %v", err)
}
```

## Placement des fichiers

Le système supporte le placement intelligent des fichiers via le champ `Path` du modèle `Env`.

### Exemples de placements courants

| Fichier | Type | Path | Description |
|---------|------|------|-------------|
| `google-services.json` | file | `android/app/google-services.json` | Configuration Firebase Android |
| `GoogleService-Info.plist` | file | `ios/Runner/GoogleService-Info.plist` | Configuration Firebase iOS |
| `.env` | file | `.env` | Variables d'environnement Flutter |
| `key.properties` | file | `android/key.properties` | Configuration signing Android |

### Format d'encodage des paths

Dans le ConfigMap Kubernetes, les paths sont encodés avec le format :
```
filename::encoded_path
```

Où `encoded_path` remplace `/` par `__`. Exemple :
```
google-services::android__app__google-services.json
```

Le script `build.sh` décode automatiquement ces paths lors du build.

## Platformes supportées

### Android

- **APK** : `BuildTarget = "apk"`
- **App Bundle (AAB)** : `BuildTarget = "aab"` ou `"appbundle"`

Fichiers générés :
- APK : `/outputs/app-{BUILD_ID}.apk`
- AAB : `/outputs/app-{BUILD_ID}.aab`

### iOS

- **BuildTarget** : `"ios"`

Fichiers générés :
- Archive : `/outputs/ios-build-{BUILD_ID}.tar.gz`

⚠️ **Note** : Les builds iOS nécessitent du code signing. Le build génère une version non signée.

### Web

- **BuildTarget** : `"web"`

Fichiers générés :
- Archive : `/outputs/web-build-{BUILD_ID}.tar.gz`

## Modes de build

- `release` : Build de production optimisé (défaut)
- `debug` : Build avec symboles de debug
- `profile` : Build pour profiling de performance

## Canaux Flutter

- `stable` : Version stable (défaut)
- `beta` : Version beta
- `dev` : Version development
- `master` : Dernière version

## Ressources Kubernetes

### Par défaut

- **CPU Request** : 1 core
- **CPU Limit** : 4 cores
- **Memory Request** : 2 Gi
- **Memory Limit** : 8 Gi
- **Storage (PVC)** : 5 Gi

Ces valeurs peuvent être ajustées dans `pod.go`.

## Gestion des artifacts

Les artifacts sont stockés dans un PersistentVolumeClaim dédié monté sur `/outputs`.

### Récupération des artifacts

**Option 1 : Copie depuis le PVC**
```bash
kubectl cp default/build-{BUILD_ID}:/outputs/app-{BUILD_ID}.apk ./app.apk
```

**Option 2 : Upload vers object storage (recommandé)**

Modifiez `build.sh` pour uploader automatiquement vers S3/MinIO/GCS :

```bash
# À la fin du build
if [ -f "$OUTPUT_FILE" ]; then
    aws s3 cp "$OUTPUT_FILE" "s3://builds/$BUILD_ID/"
    # ou
    gsutil cp "$OUTPUT_FILE" "gs://builds/$BUILD_ID/"
fi
```

## Sécurité

### Keystores

Les keystores sont stockés en base64 dans la base de données et exposés aux pods via des Secrets Kubernetes.

⚠️ **Important** :
- Chiffrez les mots de passe dans votre base de données
- Utilisez des Secrets Kubernetes chiffrés au repos
- Limitez les accès RBAC aux Secrets

### Git Credentials

Les credentials Git peuvent être passés via les variables `GIT_USERNAME` et `GIT_PASSWORD`.

💡 **Recommandation** : Utilisez des tokens d'accès personnel plutôt que des mots de passe.

## Troubleshooting

### Le pod reste en "Pending"

```bash
kubectl describe pod build-{BUILD_ID} -n default
```

Vérifiez :
- Les ressources disponibles dans le cluster
- Le PVC peut être provisionné
- L'image Docker est accessible

### Erreurs de build

```bash
kubectl logs build-{BUILD_ID} -n default
```

### Nettoyer manuellement

```bash
# Supprimer le pod
kubectl delete pod build-{BUILD_ID} -n default

# Supprimer le PVC
kubectl delete pvc build-{BUILD_ID}-artifacts -n default

# Supprimer ConfigMap et Secret
kubectl delete configmap build-{BUILD_ID}-env-files -n default
kubectl delete secret build-{BUILD_ID}-keystore -n default
```

## Migration depuis l'ancien système

L'ancien système utilisait des commandes shell inline. Pour migrer :

1. **Mettez à jour les appels** :
   ```go
   // Ancien
   kubernetes.CreateBuildPod(buildID, project, "android")

   // Nouveau
   config := kubernetes.BuildConfig{
       BuildID:  buildID,
       Project:  project,
       Platform: "android",
   }
   kubernetes.CreateBuildPod(config)
   ```

2. **Migrez vos variables d'environnement** vers le modèle `Env`

3. **Construisez et déployez la nouvelle image Docker**

## Améliorations futures

- [ ] Support des builds multi-platform simultanés
- [ ] Cache des dépendances Flutter/Gradle
- [ ] Notifications Webhook en fin de build
- [ ] Interface UI pour gérer les variables/fichiers
- [ ] Support des builds incrémentaux
- [ ] Métriques et monitoring Prometheus
- [ ] Auto-upload des artifacts vers object storage
