#!/bin/bash

# Script pour construire et publier l'image podman de build Flutter

set -e

# Configuration
IMAGE_NAME="${DOCKER_IMAGE_NAME:-flotio/flutter-build}"
IMAGE_TAG="${DOCKER_IMAGE_TAG:-latest}"
DOCKERFILE="flutter-build.Dockerfile"
REGISTRY="${DOCKER_REGISTRY:-}"

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Flutter Build Image Builder${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Vérifier que podman est installé
if ! command -v podman-remote-static-linux_amd64 &> /dev/null; then
    echo -e "${RED}Error: podman is not installed${NC}"
    exit 1
fi

echo -e "${GREEN}Configuration:${NC}"
echo "  Image Name: $IMAGE_NAME"
echo "  Tag: $IMAGE_TAG"
echo "  Dockerfile: $DOCKERFILE"
if [ -n "$REGISTRY" ]; then
    echo "  Registry: $REGISTRY"
    FULL_IMAGE_NAME="$REGISTRY/$IMAGE_NAME:$IMAGE_TAG"
else
    FULL_IMAGE_NAME="$IMAGE_NAME:$IMAGE_TAG"
fi
echo "  Full Image: $FULL_IMAGE_NAME"
echo ""

# Vérifier que le Dockerfile existe
if [ ! -f "$DOCKERFILE" ]; then
    echo -e "${RED}Error: Dockerfile '$DOCKERFILE' not found${NC}"
    exit 1
fi

# Vérifier que build.sh existe
if [ ! -f "build.sh" ]; then
    echo -e "${RED}Error: build.sh not found${NC}"
    exit 1
fi

# Build de l'image
echo -e "${YELLOW}Building podman image...${NC}"
podman-remote-static-linux_amd64 build \
    -f "$DOCKERFILE" \
    -t "$FULL_IMAGE_NAME" \
    --progress=plain \
    .

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: podman build failed${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Image built successfully${NC}"
echo ""

# Afficher les informations sur l'image
IMAGE_SIZE=$(podman-remote-static-linux_amd64 images "$FULL_IMAGE_NAME" --format "{{.Size}}")
echo -e "${GREEN}Image Information:${NC}"
echo "  Name: $FULL_IMAGE_NAME"
echo "  Size: $IMAGE_SIZE"
echo ""

# Demander si on doit pousser l'image
read -p "Do you want to push the image to the registry? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}Pushing image to registry...${NC}"

    # Login au registry si nécessaire
    if [ -n "$REGISTRY" ] && [ -n "$DOCKER_USERNAME" ] && [ -n "$DOCKER_PASSWORD" ]; then
        echo "Logging in to registry..."
        echo "$DOCKER_PASSWORD" | podman login "$REGISTRY" -u "$DOCKER_USERNAME" --password-stdin
    fi

    podman-remote-static-linux_amd64 push "$FULL_IMAGE_NAME"

    if [ $? -ne 0 ]; then
        echo -e "${RED}Error: podman push failed${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ Image pushed successfully${NC}"
fi

echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${GREEN}Done!${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo "To use this image, update your Kubernetes configuration:"
echo "  export FLUTTER_BUILD_IMAGE=$FULL_IMAGE_NAME"
echo ""
echo "Or set it in your environment variables:"
echo "  FLUTTER_BUILD_IMAGE=$FULL_IMAGE_NAME"
