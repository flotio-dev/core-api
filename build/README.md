# Flutter Build System - Build Directory

Ce dossier contient les fichiers nécessaires pour construire l'image Docker du système de build Flutter.

## Fichiers

- **`flutter-build.Dockerfile`** : Dockerfile pour créer l'image de build
- **`build.sh`** : Script de build exécuté dans le conteneur
- **`build-image.sh`** : Script helper pour construire et publier l'image Docker
- **`.dockerignore`** : Fichiers à exclure du contexte Docker

## Construction de l'image

### Méthode simple

```bash
cd build/
./build-image.sh
```

Le script va :
1. Construire l'image Docker
2. Afficher les informations sur l'image
3. Demander si vous voulez la pousser vers un registry

### Méthode manuelle

```bash
cd build/
docker build -f flutter-build.Dockerfile -t flotio/flutter-build:latest .
```

### Configuration personnalisée

Vous pouvez personnaliser la construction avec des variables d'environnement :

```bash
# Nom de l'image personnalisé
export DOCKER_IMAGE_NAME="my-company/flutter-build"

# Tag personnalisé
export DOCKER_IMAGE_TAG="v1.0.0"

# Registry privé
export DOCKER_REGISTRY="registry.example.com"

# Credentials pour le registry
export DOCKER_USERNAME="myuser"
export DOCKER_PASSWORD="mypassword"

./build-image.sh
```

## Publication sur un registry

### Docker Hub

```bash
docker login
docker tag flotio/flutter-build:latest yourusername/flutter-build:latest
docker push yourusername/flutter-build:latest
```

### Registry privé

```bash
docker login registry.example.com
docker tag flotio/flutter-build:latest registry.example.com/flutter-build:latest
docker push registry.example.com/flutter-build:latest
```

### Google Container Registry (GCR)

```bash
gcloud auth configure-docker
docker tag flotio/flutter-build:latest gcr.io/your-project/flutter-build:latest
docker push gcr.io/your-project/flutter-build:latest
```

### Amazon ECR

```bash
aws ecr get-login-password --region region | docker login --username AWS --password-stdin aws_account_id.dkr.ecr.region.amazonaws.com
docker tag flotio/flutter-build:latest aws_account_id.dkr.ecr.region.amazonaws.com/flutter-build:latest
docker push aws_account_id.dkr.ecr.region.amazonaws.com/flutter-build:latest
```

## Utilisation dans Kubernetes

Une fois l'image publiée, configurez votre système pour l'utiliser :

```bash
export FLUTTER_BUILD_IMAGE="votreregistry/flutter-build:latest"
```

Ou dans votre code Go :

```go
os.Setenv("FLUTTER_BUILD_IMAGE", "votreregistry/flutter-build:latest")
```

## Personnalisation du Dockerfile

### Changer la version de Flutter

Éditez `flutter-build.Dockerfile` :

```dockerfile
ENV FLUTTER_VERSION=3.16.0  # Version spécifique
# ou
ENV FLUTTER_VERSION=stable  # Canal stable
ENV FLUTTER_VERSION=beta    # Canal beta
```

### Ajouter des outils supplémentaires

```dockerfile
RUN apt-get update && apt-get install -y \
    your-tool \
    another-tool \
    && rm -rf /var/lib/apt/lists/*
```

### Changer la version d'Android SDK

```dockerfile
ENV ANDROID_BUILD_TOOLS_VERSION=33.0.0
ENV ANDROID_PLATFORMS_VERSION=33
```

## Personnalisation du script de build

Le script `build.sh` supporte plusieurs variables d'environnement. Voir la documentation complète dans `docs/FLUTTER_BUILD_SYSTEM.md`.

### Ajouter des étapes de build

Éditez `build.sh` et ajoutez vos étapes :

```bash
# Avant le build
echo "Running custom pre-build steps..."
# Vos commandes ici

# Après le build
echo "Running custom post-build steps..."
# Upload vers S3, notification, etc.
```

## Tests locaux

Pour tester l'image localement sans Kubernetes :

```bash
# Construire l'image
docker build -f flutter-build.Dockerfile -t flutter-build-test .

# Exécuter un build de test
docker run --rm \
  -e GIT_REPO="https://github.com/flutter/flutter.git" \
  -e PLATFORM="android" \
  -e BUILD_ID="test-1" \
  -e BUILD_FOLDER="examples/hello_world" \
  -v $(pwd)/outputs:/outputs \
  flutter-build-test
```

## Dépannage

### L'image est trop grosse

L'image complète fait environ 4-6 GB en raison de l'Android SDK. Pour réduire :

1. Utilisez des builds multi-stage
2. Supprimez les composants SDK non utilisés
3. Utilisez une image de base plus légère

### Le build échoue

Vérifiez :

```bash
# Construire sans cache
docker build --no-cache -f flutter-build.Dockerfile -t flutter-build-test .

# Voir les logs complets
docker build --progress=plain -f flutter-build.Dockerfile -t flutter-build-test .
```

### Le script build.sh n'est pas trouvé

Assurez-vous que :
- `build.sh` est dans le même dossier que le Dockerfile
- Le fichier a les permissions d'exécution (`chmod +x build.sh`)
- Le `.dockerignore` n'exclut pas `build.sh`

## Mise à jour

Pour mettre à jour l'image avec les dernières dépendances :

```bash
cd build/

# Reconstruire sans cache
docker build --no-cache --pull -f flutter-build.Dockerfile -t flotio/flutter-build:latest .

# Pousser la nouvelle version
docker push flotio/flutter-build:latest
```

## Versions multiples

Vous pouvez maintenir plusieurs versions de l'image :

```bash
# Stable
docker build -f flutter-build.Dockerfile -t flotio/flutter-build:stable .

# Beta
docker build -f flutter-build.Dockerfile -t flotio/flutter-build:beta \
  --build-arg FLUTTER_VERSION=beta .

# Version spécifique
docker build -f flutter-build.Dockerfile -t flotio/flutter-build:3.16.0 \
  --build-arg FLUTTER_VERSION=3.16.0 .
```
