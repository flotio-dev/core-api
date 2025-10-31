# Docker Image Size Optimization Guide

## 🎯 Optimizations Applied

### 1. **Package Management Optimizations**
- ✅ Used `--no-install-recommends` flag to avoid unnecessary dependencies
- ✅ Switched from `-dev` packages to runtime versions only (libgtk-3-0 instead of libgtk-3-dev)
- ✅ Used `openjdk-headless` instead of full JDK (saves ~200MB)
- ✅ Removed apt cache and lists after installation
- ✅ Cleaned `/tmp/` and `/var/tmp/` directories

### 2. **Android SDK Optimizations**
- ✅ Removed unnecessary Android SDK components:
  - `extras;google;google_play_services` (not needed for build)
  - `extras;google;m2repository` (not needed for build)
  - `extras;android;m2repository` (not needed for build)
- ✅ Cleaned up Android SDK to remove:
  - `/tools` directory (deprecated)
  - `/emulator` (not needed for CI builds)
  - `/system-images` (emulator images)
  - `/sources` (source code references)
  - NDK prebuilt files for other platforms
  - Unnecessary tools like simpleperf and shader-tools
- ✅ Removed backup files (*.jar.orig, *.zip)

### 3. **Flutter Optimizations**
- ✅ Used `--depth 1 --single-branch` for git clone (saves ~500MB)
- ✅ Precached only Android artifacts (`--android --no-ios --no-web --no-linux --no-windows --no-macos`)
- ✅ Removed Flutter's `.git` history
- ✅ Removed Flutter artifacts for other platforms (iOS, macOS, Linux, Windows, Fuchsia)
- ✅ Deleted examples and benchmarks
- ✅ Removed all `.md` and `.txt` documentation files

### 4. **System-level Cleanup**
- ✅ Final `apt-get autoremove` to remove orphaned packages
- ✅ Cleaned apt cache and temporary files
- ✅ Cleaned journal logs with `journalctl --vacuum-time=1s`
- ✅ Removed root cache directories

## 📊 Expected Size Reduction

| Component | Before | After | Savings |
|-----------|--------|-------|---------|
| Base Ubuntu | 77 MB | 77 MB | 0 MB |
| Java (full JDK → headless) | 450 MB | 250 MB | **200 MB** |
| Android SDK (with extras) | 1.8 GB | 1.2 GB | **600 MB** |
| Flutter (full clone) | 1.2 GB | 600 MB | **600 MB** |
| System packages (-dev versions) | 300 MB | 150 MB | **150 MB** |
| **Total Estimated** | **~3.8 GB** | **~2.3 GB** | **~1.5 GB (40%)** |

## 🚀 Additional Optimization Options

### Option 1: Multi-stage Build (Advanced)
If you want to go even further, consider a multi-stage build where you only copy the final artifacts:

```dockerfile
FROM ubuntu:22.04 AS builder
# ... all build tools ...

FROM ubuntu:22.04 AS runtime
# Copy only necessary runtime files
COPY --from=builder /opt/flutter /opt/flutter
COPY --from=builder /opt/android-sdk /opt/android-sdk
# ... minimal runtime dependencies ...
```

**Potential savings**: Additional 200-300 MB

### Option 2: Alpine-based Image
Use Alpine Linux instead of Ubuntu for an even smaller base:

```dockerfile
FROM alpine:3.18
```

**Pros**: Base image only 7 MB instead of 77 MB
**Cons**: More compatibility issues, requires musl libc instead of glibc
**Recommendation**: Not recommended for Flutter/Android builds due to compatibility issues

### Option 3: Use Pre-built Flutter Docker Image
Consider using CircleCI's official Flutter image as a base:

```dockerfile
FROM circleci/flutter:3.x.x
```

**Pros**: Maintained by the community, regularly updated
**Cons**: Less control over customization, may include unnecessary tools

## 📝 Build Command Recommendations

### Standard Build
```bash
docker build -t flotio/flutter-build:latest -f flutter-build.Dockerfile .
```

### Build with BuildKit (faster, better caching)
```bash
DOCKER_BUILDKIT=1 docker build \
  --progress=plain \
  -t flotio/flutter-build:latest \
  -f flutter-build.Dockerfile .
```

### Build with Compression (smaller final image)
```bash
docker build \
  --compress \
  --squash \
  -t flotio/flutter-build:latest \
  -f flutter-build.Dockerfile .
```

**Note**: `--squash` requires experimental features enabled

## 🔍 Verify Image Size

Check the final image size:
```bash
docker images flotio/flutter-build:latest
```

Analyze layers to find optimization opportunities:
```bash
docker history flotio/flutter-build:latest
```

Use dive for detailed layer analysis:
```bash
# Install dive: https://github.com/wagoodman/dive
dive flotio/flutter-build:latest
```

## ⚡ Runtime Performance Tips

1. **Use volume caching for dependencies**:
   ```yaml
   volumes:
     - gradle-cache:/root/.gradle
     - flutter-pub-cache:/root/.pub-cache
   ```

2. **Enable BuildKit caching**:
   ```bash
   export DOCKER_BUILDKIT=1
   ```

3. **Use local registry for faster pulls**:
   ```bash
   docker pull flotio/flutter-build:latest
   docker tag flotio/flutter-build:latest localhost:5000/flutter-build
   docker push localhost:5000/flutter-build
   ```

## 🎯 Best Practices Applied

- ✅ Combined RUN commands to reduce layers
- ✅ Cleaned up in the same layer as installation
- ✅ Used specific package versions
- ✅ Minimized installed packages
- ✅ Removed build-time dependencies
- ✅ Used `.dockerignore` to exclude unnecessary files
- ✅ Created non-root user for security

## 📈 Monitoring & Maintenance

### Regular Maintenance
- Update base image: `FROM ubuntu:22.04` → `FROM ubuntu:24.04`
- Update Flutter version periodically
- Review and remove unused dependencies
- Monitor image size over time

### Automated Size Checks
Add to CI/CD pipeline:
```bash
#!/bin/bash
IMAGE_SIZE=$(docker images flotio/flutter-build:latest --format "{{.Size}}")
MAX_SIZE="2.5GB"
if [ "$IMAGE_SIZE" > "$MAX_SIZE" ]; then
  echo "⚠️  Image size exceeded: $IMAGE_SIZE"
  exit 1
fi
```

## 🆘 Troubleshooting

### Issue: Build fails with "package not found"
**Solution**: You may have removed a required dependency. Re-add it to the apt-get install list.

### Issue: Flutter plugin build fails
**Solution**: Some plugins need `-dev` packages. Add them back selectively:
```bash
apt-get install -y libgtk-3-dev  # for flutter_webview
apt-get install -y libsqlite3-dev  # for sqflite
```

### Issue: Out of disk space during build
**Solution**:
1. Clean up Docker: `docker system prune -a`
2. Increase Docker disk space allocation
3. Use multi-stage build to reduce intermediate layers

---

**Last updated**: October 31, 2025
**Image version**: v2.0 (optimized)
**Maintained by**: Flotio Dev Team
