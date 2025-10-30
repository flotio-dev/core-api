# Configuration Android pour le signing avec Keystore

## Pour que le système de build fonctionne avec le signing automatique, votre projet Flutter doit être configuré correctement.

## 1. Fichier android/key.properties

Ce fichier est généré automatiquement par le script de build. Il contient :

```properties
storePassword=your-store-password
keyPassword=your-key-password
keyAlias=your-key-alias
storeFile=/keystore/keystore.jks
```

## 2. Configuration android/app/build.gradle

Ajoutez ce code **AVANT** le bloc `android {` :

```gradle
def keystoreProperties = new Properties()
def keystorePropertiesFile = rootProject.file('key.properties')
if (keystorePropertiesFile.exists()) {
    keystoreProperties.load(new FileInputStream(keystorePropertiesFile))
}
```

Dans le bloc `android {`, ajoutez la configuration `signingConfigs` **AVANT** `buildTypes` :

```gradle
android {
    // ... autres configurations ...

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
            // Ajouter cette ligne pour utiliser la config de signing
            signingConfig signingConfigs.release

            // Autres configurations release...
            minifyEnabled true
            shrinkResources true
        }

        debug {
            // Le debug utilise le keystore debug par défaut
        }
    }
}
```

## 3. Fichier build.gradle complet (exemple)

```gradle
def localProperties = new Properties()
def localPropertiesFile = rootProject.file('local.properties')
if (localPropertiesFile.exists()) {
    localPropertiesFile.withReader('UTF-8') { reader ->
        localProperties.load(reader)
    }
}

def flutterRoot = localProperties.getProperty('flutter.sdk')
if (flutterRoot == null) {
    throw new GradleException("Flutter SDK not found. Define location with flutter.sdk in the local.properties file.")
}

def flutterVersionCode = localProperties.getProperty('flutter.versionCode')
if (flutterVersionCode == null) {
    flutterVersionCode = '1'
}

def flutterVersionName = localProperties.getProperty('flutter.versionName')
if (flutterVersionName == null) {
    flutterVersionName = '1.0'
}

// Configuration du keystore
def keystoreProperties = new Properties()
def keystorePropertiesFile = rootProject.file('key.properties')
if (keystorePropertiesFile.exists()) {
    keystoreProperties.load(new FileInputStream(keystorePropertiesFile))
}

apply plugin: 'com.android.application'
apply plugin: 'kotlin-android'
apply from: "$flutterRoot/packages/flutter_tools/gradle/flutter.gradle"

android {
    compileSdkVersion 34
    ndkVersion flutter.ndkVersion

    compileOptions {
        sourceCompatibility JavaVersion.VERSION_1_8
        targetCompatibility JavaVersion.VERSION_1_8
    }

    kotlinOptions {
        jvmTarget = '1.8'
    }

    sourceSets {
        main.java.srcDirs += 'src/main/kotlin'
    }

    defaultConfig {
        applicationId "com.example.myapp"
        minSdkVersion 21
        targetSdkVersion 34
        versionCode flutterVersionCode.toInteger()
        versionName flutterVersionName
    }

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

            minifyEnabled true
            shrinkResources true
            proguardFiles getDefaultProguardFile('proguard-android-optimize.txt'), 'proguard-rules.pro'
        }

        debug {
            applicationIdSuffix ".debug"
            debuggable true
        }
    }
}

flutter {
    source '../..'
}

dependencies {
    implementation "org.jetbrains.kotlin:kotlin-stdlib-jdk7:$kotlin_version"
}
```

## 4. Vérification

Après avoir configuré votre projet, testez localement :

```bash
# Créer un fichier key.properties de test
cat > android/key.properties << EOF
storePassword=android
keyPassword=android
keyAlias=androiddebugkey
storeFile=/path/to/debug.keystore
EOF

# Tester le build
flutter build apk --release
```

## 5. Notes importantes

### Keystore Debug vs Release

- **Debug** : Utilisé automatiquement par Flutter, pas besoin de configuration
- **Release** : Nécessite la configuration ci-dessus

### Format du keystore

Le système accepte les formats :
- `.jks` (Java KeyStore - recommandé)
- `.keystore` (ancien format)

### Génération d'un keystore

Si vous n'avez pas de keystore :

```bash
keytool -genkey -v -keystore release.jks \
  -keyalg RSA -keysize 2048 -validity 10000 \
  -alias key \
  -storetype JKS
```

Ou avec les nouveaux outils :

```bash
keytool -genkey -v -keystore release.jks \
  -keyalg RSA -keysize 2048 -validity 10000 \
  -alias key
```

### Sécurité du keystore

⚠️ **IMPORTANT** :
- Ne commitez JAMAIS votre keystore dans Git
- Ajoutez `*.jks`, `*.keystore`, `key.properties` dans `.gitignore`
- Sauvegardez votre keystore en lieu sûr
- Si vous perdez votre keystore, vous ne pourrez plus publier de mises à jour sur le Play Store

### Fichier .gitignore recommandé

```gitignore
# Android signing
*.jks
*.keystore
key.properties

# Secrets
google-services.json
GoogleService-Info.plist
```

## 6. Troubleshooting

### Erreur : "Keystore file not found"

Le chemin dans `key.properties` doit être absolu ou relatif au dossier `android/app/`.

### Erreur : "Failed to read key from keystore"

Vérifiez que :
- Le mot de passe du store est correct
- L'alias de la clé existe dans le keystore
- Le mot de passe de la clé est correct

```bash
# Lister les alias dans un keystore
keytool -list -v -keystore release.jks
```

### Build non signé en release

Si le build réussit mais l'APK n'est pas signé :
- Vérifiez que `key.properties` existe
- Vérifiez que `signingConfig signingConfigs.release` est dans `buildTypes.release`
- Vérifiez les logs Gradle pour voir si la config de signing est chargée

### Vérifier qu'un APK est signé

```bash
# Vérifier la signature d'un APK
jarsigner -verify -verbose -certs app-release.apk

# Ou avec apksigner (Android SDK build-tools)
apksigner verify --verbose app-release.apk
```
