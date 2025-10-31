# Configuration d'un utilisateur Admin Kubernetes

Ce guide explique comment créer un utilisateur administrateur avec accès complet au cluster Kubernetes et comment récupérer sa clé API.

## Prérequis

- Accès au cluster Kubernetes avec des privilèges admin
- kubectl installé et configuré
- Accès au namespace où l'application core-api est déployée

## Étapes de configuration

### 1. Créer un ServiceAccount pour l'admin

```bash
kubectl create serviceaccount admin-user -n kube-system
```

### 2. Créer un ClusterRoleBinding avec le rôle cluster-admin

```bash
kubectl create clusterrolebinding admin-user-binding \
  --clusterrole=cluster-admin \
  --serviceaccount=kube-system:admin-user
```

### 3. Créer un Secret pour le ServiceAccount (Kubernetes 1.24+)

Depuis Kubernetes 1.24, les secrets ne sont plus créés automatiquement. Créez un fichier `admin-secret.yaml` :

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: admin-user-secret
  namespace: kube-system
  annotations:
    kubernetes.io/service-account.name: admin-user
type: kubernetes.io/service-account-token
```

Appliquez-le :

```bash
kubectl apply -f admin-secret.yaml
```

### 4. Récupérer le token (clé API)

#### Méthode 1 : Via le Secret

```bash
kubectl get secret admin-user-secret -n kube-system -o jsonpath='{.data.token}' | base64 --decode
```

#### Méthode 2 : Via kubectl (Kubernetes 1.24+)

```bash
kubectl create token admin-user -n kube-system --duration=8760h
```

**Note**: Cette méthode génère un token avec une durée de validité limitée (ici 1 an).

### 5. Récupérer les informations du cluster

#### URL du serveur API

```bash
kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}'
```

#### Certificat CA du cluster

```bash
kubectl get secret admin-user-secret -n kube-system -o jsonpath='{.data.ca\.crt}'
```

### 6. Configurer l'application core-api

Ajoutez ces informations dans les variables d'environnement ou la configuration de core-api :

```bash
# Token d'authentification
KUBERNETES_TOKEN=<token-récupéré-étape-4>

# URL du serveur API (si différent de l'URL in-cluster)
KUBERNETES_API_URL=<url-serveur-api>

# Certificat CA (optionnel, pour connexion externe)
KUBERNETES_CA_CERT=<certificat-ca>
```

## Configuration kubectl pour l'utilisateur admin

Pour utiliser ce compte avec kubectl localement :

```bash
# Définir les credentials
kubectl config set-credentials admin-user --token=<votre-token>

# Créer un contexte
kubectl config set-context admin-context \
  --cluster=<nom-de-votre-cluster> \
  --user=admin-user

# Utiliser le contexte
kubectl config use-context admin-context
```

## Vérification des permissions

Testez l'accès avec le token :

```bash
# Lister tous les pods dans tous les namespaces
kubectl --token=<votre-token> get pods --all-namespaces

# Vérifier les permissions
kubectl --token=<votre-token> auth can-i --list
```

## Configuration dans core-api

### Exemple d'utilisation dans le code Go

```go
import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
)

func createKubernetesClient() (*kubernetes.Clientset, error) {
    config := &rest.Config{
        Host:        os.Getenv("KUBERNETES_API_URL"),
        BearerToken: os.Getenv("KUBERNETES_TOKEN"),
        TLSClientConfig: rest.TLSClientConfig{
            Insecure: false, // Mettre true pour dev, false en prod
        },
    }

    return kubernetes.NewForConfig(config)
}
```

### Configuration in-cluster (recommandée)

Si core-api tourne dans le cluster, utilisez la configuration in-cluster :

```go
import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
)

func createInClusterClient() (*kubernetes.Clientset, error) {
    config, err := rest.InClusterConfig()
    if err != nil {
        return nil, err
    }

    return kubernetes.NewForConfig(config)
}
```

**Important** : Dans ce cas, le ServiceAccount `admin-user` doit être assigné au pod core-api :

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: core-api
  namespace: kube-system
spec:
  serviceAccountName: admin-user
  containers:
  - name: core-api
    image: your-image
```

## Sécurité

⚠️ **Attention** : Le rôle `cluster-admin` donne un accès complet au cluster. Considérations de sécurité :

1. **Rotation des tokens** : Régénérez les tokens régulièrement
2. **Durée de vie limitée** : Utilisez des tokens avec expiration
3. **Audit** : Activez l'audit Kubernetes pour tracer les actions
4. **RBAC minimal** : Si possible, créez un rôle avec permissions minimales requises
5. **Secrets management** : Utilisez un gestionnaire de secrets (Vault, Sealed Secrets, etc.)

## Alternative : Rôle avec permissions minimales

Si vous n'avez pas besoin d'un accès complet, créez un rôle personnalisé :

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: core-api-role
rules:
- apiGroups: [""]
  resources: ["pods", "pods/log", "services"]
  verbs: ["get", "list", "watch", "create", "update", "delete"]
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch", "create", "update", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: core-api-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: core-api-role
subjects:
- kind: ServiceAccount
  name: admin-user
  namespace: kube-system
```

## Dépannage

### Token invalide ou expiré

Régénérez le token :

```bash
kubectl delete secret admin-user-secret -n kube-system
kubectl apply -f admin-secret.yaml
```

### Permissions insuffisantes

Vérifiez le ClusterRoleBinding :

```bash
kubectl get clusterrolebinding admin-user-binding -o yaml
```

### Connexion refusée

Vérifiez que l'URL du serveur API est accessible :

```bash
curl -k $(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
```

## Ressources

- [Kubernetes RBAC Documentation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [Service Accounts](https://kubernetes.io/docs/concepts/security/service-accounts/)
- [client-go Authentication](https://github.com/kubernetes/client-go/blob/master/examples/out-of-cluster-client-configuration/main.go)
