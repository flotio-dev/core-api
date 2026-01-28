# Guide de démarrage rapide - Système de build Flutter

Ce guide vous permet de démarrer rapidement avec le nouveau système de build Flutter.

## 🚀 Démarrage en 5 étapes

### Étape 1 : Migration de la base de données

Deux options disponibles :

**Option A - Avec GORM (recommandé) :**
```bash
cd migrations/
go run migrate.go
```

**Option B - SQL direct :**
```bash
psql -U your_user -d your_database -f migrations/add_flutter_build_support.sql
```

### Étape 2 : Construire l'image Docker

```bash
cd build/
./build-image.sh
```

Ou manuellement :
```bash
docker build -f flutter-build.Dockerfile -t flotio/flutter-build:latest .
```

### Étape 3 : Publier l'image (optionnel pour prod)

```bash
# Docker Hub
docker push flotio/flutter-build:latest

# Ou registry privé
docker tag flotio/flutter-build:latest registry.example.com/flutter-build:latest
docker push registry.example.com/flutter-build:latest
```

### Étape 4 : Configurer l'environnement

```bash
# Pour Kubernetes local
export K8S_NAMESPACE=default
export FLUTTER_BUILD_IMAGE=flotio/flutter-build:latest

# Pour Kubernetes distant
export KUBECTL_API=https://your-k8s-api:6443
export KUBECTL_TOKEN=your-token
```

### Étape 5 : Lancer votre premier build

Utilisez l'exemple dans `examples/flutter_build_example.go` ou :

```go
import "github.com/flotio-dev/core-api/pkg/kubernetes"

// Configuration du build
config := kubernetes.BuildConfig{
    BuildID:        1,
    Project:        project,
    Platform:       "android",
    BuildMode:      "release",
    BuildTarget:    "apk",
    FlutterChannel: "stable",
    GitBranch:      "main",
}

// Lancer le build
err := kubernetes.CreateBuildPod(config)
```

## 📝 Configuration d'un projet

### 1. Variables d'environnement

```go
env := db.Env{
    ProjectID: 1,
    Key:       "API_KEY",
    Value:     "your-api-key",
    Type:      "env",
}
db.DB.Create(&env)
```

### 2. Fichiers de configuration

```go
// google-services.json
googleServices := db.Env{
    ProjectID: 1,
    Key:       "google-services.json",
    Value:     `{"project_info": {...}}`,
    Type:      "file",
    Path:      "android/app/google-services.json",
    IsBase64:  false,
}
db.DB.Create(&googleServices)
```

### 3. Keystore Android

```go
import "encoding/base64"
import "io/ioutil"

keystoreBytes, _ := ioutil.ReadFile("release.jks")
keystoreB64 := base64.StdEncoding.EncodeToString(keystoreBytes)

keystore := db.Keystore{
    ProjectID:     1,
    Name:          "Production",
    KeystoreFile:  keystoreB64,
    StorePassword: "password",
    KeyAlias:      "key",
    KeyPassword:   "password",
    IsActive:      true,
}
db.DB.Create(&keystore)
```

## 🔧 Configuration du projet Flutter

### Android - build.gradle

Ajoutez dans `android/app/build.gradle` :

```gradle
def keystoreProperties = new Properties()
def keystorePropertiesFile = rootProject.file('key.properties')
if (keystorePropertiesFile.exists()) {
    keystoreProperties.load(new FileInputStream(keystorePropertiesFile))
}

android {
    signingConfigs {
        release {
            if (keystorePropertiesFile.exists()) {
                keyAlias keystoreProperties['keyAlias']
                keyPassword keystoreProperties['keyPassword']
                storeFile file(keystoreProperties['storeFile'])
                storePassword keystoreProperties['storePassword']
            }
        }
    }

    buildTypes {
        release {
            signingConfig signingConfigs.release
        }
    }
}
```

Voir `docs/ANDROID_SIGNING_SETUP.md` pour plus de détails.

## 🎯 Cas d'usage courants

### Build Android APK

```go
config := kubernetes.BuildConfig{
    BuildID:     buildID,
    Project:     project,
    Platform:    "android",
    BuildTarget: "apk",
}
kubernetes.CreateBuildPod(config)
```

### Build Android App Bundle (AAB)

```go
config := kubernetes.BuildConfig{
    BuildID:     buildID,
    Project:     project,
    Platform:    "android",
    BuildTarget: "aab",
}
kubernetes.CreateBuildPod(config)
```

### Build avec canal Flutter spécifique

```go
config := kubernetes.BuildConfig{
    BuildID:        buildID,
    Project:        project,
    Platform:       "android",
    FlutterChannel: "beta", // ou "dev", "master"
}
kubernetes.CreateBuildPod(config)
```

### Build avec authentification Git

```go
config := kubernetes.BuildConfig{
    BuildID:     buildID,
    Project:     project,
    Platform:    "android",
    GitUsername: "user",
    GitPassword: "token",
}
kubernetes.CreateBuildPod(config)
```

## 📊 Suivi d'un build

### Vérifier le statut

```go
status, err := kubernetes.GetPodStatus(buildID)
fmt.Printf("Status: %s\n", status)
// Retourne: Pending, Running, Succeeded, Failed
```

### Récupérer les logs

```go
logs, err := kubernetes.GetPodLogs(buildID)
for _, line := range logs {
    fmt.Print(line)
}
```

### Streamer les logs en temps réel

```go
logChan := make(chan string)
go kubernetes.StreamPodLogs(buildID, logChan)

for log := range logChan {
    fmt.Print(log)
    // Sauvegarder dans la DB, envoyer via WebSocket, etc.
}
```

## 🧹 Nettoyage

```go
config, _ := kubernetes.getKubernetesConfig()
clientset, _ := kubernetes.NewForConfig(config)
namespace := kubernetes.getNamespace()

kubernetes.DeleteBuildResources(clientset, buildID, namespace)
```

## 📱 Fichiers courants

| Fichier | Type | Path | Usage |
|---------|------|------|-------|
| `google-services.json` | file | `android/app/google-services.json` | Firebase Android |
| `GoogleService-Info.plist` | file | `ios/Runner/GoogleService-Info.plist` | Firebase iOS |
| `.env` | file | `.env` | Variables d'environnement |
| `keystore.jks` | keystore | - | Signature Android (via modèle Keystore) |

## 🐛 Dépannage rapide

### Le pod reste en Pending

```bash
kubectl describe pod build-{BUILD_ID}
```

Vérifiez les ressources disponibles.

### Erreur "Image not found"

Vérifiez que `FLUTTER_BUILD_IMAGE` pointe vers la bonne image :
```bash
echo $FLUTTER_BUILD_IMAGE
```

### Erreur de build

Consultez les logs :
```bash
kubectl logs build-{BUILD_ID}
```

### Nettoyage manuel

```bash
kubectl delete pod build-{BUILD_ID}
kubectl delete pvc build-{BUILD_ID}-artifacts
kubectl delete configmap build-{BUILD_ID}-env-files
kubectl delete secret build-{BUILD_ID}-keystore
```

## 📚 Documentation complète

- **Système complet** : `docs/FLUTTER_BUILD_SYSTEM.md`
- **Configuration Android** : `docs/ANDROID_SIGNING_SETUP.md`
- **Construction de l'image** : `build/README.md`
- **Exemples de code** : `examples/flutter_build_example.go`

## 🔐 Sécurité

**Important :**
- ✅ Chiffrez les mots de passe des keystores en base de données
- ✅ Utilisez des tokens Git plutôt que des mots de passe
- ✅ Limitez les accès RBAC Kubernetes aux pods de build
- ✅ Utilisez des Secrets Kubernetes chiffrés au repos
- ❌ Ne commitez jamais les keystores dans Git

## 🎉 Vous êtes prêt !

Le système est maintenant configuré. Commencez par :

1. Configurer un projet de test
2. Lancer un build
3. Surveiller les logs
4. Récupérer l'APK/AAB généré

Pour plus d'informations, consultez la documentation complète dans `docs/`.

## 💬 Besoin d'aide ?

- Consultez les exemples dans `examples/`
- Vérifiez les logs Kubernetes : `kubectl logs`
- Testez l'image localement : `docker run`
- Examinez le script de build : `build/build.sh`
