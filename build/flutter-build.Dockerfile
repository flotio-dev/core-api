# Flutter Build Container - Production Ready (Optimized)
# Includes Android SDK, Java, Flutter and all necessary build tools
# Multi-architecture support (amd64/arm64)

FROM ubuntu:22.04 AS builder

# Avoid prompts from apt
ENV DEBIAN_FRONTEND=noninteractive

# Define versions
ENV FLUTTER_VERSION=3.35.7
ENV ANDROID_SDK_VERSION=9477386
ENV ANDROID_BUILD_TOOLS_VERSION=34.0.0
ENV ANDROID_PLATFORMS_VERSION=34
ENV JAVA_VERSION=17

# Detect architecture for Android SDK
ARG TARGETARCH
ENV TARGETARCH=${TARGETARCH}

# Install base dependencies (minimal set + build essentials)
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    git \
    unzip \
    xz-utils \
    zip \
    libglu1-mesa \
    openjdk-${JAVA_VERSION}-jdk-headless \
    wget \
    ca-certificates \
    # Additional libraries for Flutter/Android builds
    clang \
    cmake \
    ninja-build \
    pkg-config \
    libgtk-3-0 \
    liblzma5 \
    libstdc++6 \
    # Libraries for certain Flutter plugins
    libglib2.0-0 \
    libsqlite3-0 \
    libgtk-3-dev \
    libsqlite3-dev \
    # For file operations
    file \
    # For build performance
    ccache \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /tmp/* \
    && rm -rf /var/tmp/*

# Set Java environment
ENV JAVA_HOME=/usr/lib/jvm/java-${JAVA_VERSION}-openjdk-${TARGETARCH}
ENV PATH=$PATH:$JAVA_HOME/bin

# Install Android SDK (architecture-aware)
ENV ANDROID_HOME=/opt/android-sdk
ENV ANDROID_SDK_ROOT=$ANDROID_HOME
ENV PATH=$PATH:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$ANDROID_HOME/build-tools/${ANDROID_BUILD_TOOLS_VERSION}

RUN mkdir -p $ANDROID_HOME/cmdline-tools && \
    cd $ANDROID_HOME/cmdline-tools && \
    wget -q https://dl.google.com/android/repository/commandlinetools-linux-${ANDROID_SDK_VERSION}_latest.zip && \
    unzip -q commandlinetools-linux-${ANDROID_SDK_VERSION}_latest.zip && \
    rm commandlinetools-linux-${ANDROID_SDK_VERSION}_latest.zip && \
    mv cmdline-tools latest && \
    # Clean up unnecessary files
    rm -rf $ANDROID_HOME/cmdline-tools/latest/NOTICE.txt

# Accept licenses and install Android components (minimal set)
RUN yes | sdkmanager --licenses && \
    sdkmanager --install \
    "platform-tools" \
    "platforms;android-${ANDROID_PLATFORMS_VERSION}" \
    "build-tools;${ANDROID_BUILD_TOOLS_VERSION}" \
    "ndk;25.1.8937393" && \
    # Clean up Android SDK to reduce size
    rm -rf $ANDROID_HOME/tools \
    && rm -rf $ANDROID_HOME/emulator \
    && rm -rf $ANDROID_HOME/system-images \
    && rm -rf $ANDROID_HOME/sources \
    && rm -rf $ANDROID_HOME/ndk/*/prebuilt/android-* \
    && rm -rf $ANDROID_HOME/ndk/*/simpleperf \
    && rm -rf $ANDROID_HOME/ndk/*/shader-tools \
    && find $ANDROID_HOME -name "*.jar.orig" -delete \
    && find $ANDROID_HOME -name "*.zip" -delete

# Install Flutter
ENV FLUTTER_HOME=/opt/flutter
ENV PATH=$PATH:$FLUTTER_HOME/bin

RUN git clone https://github.com/flutter/flutter.git -b ${FLUTTER_VERSION} $FLUTTER_HOME && \
    cd $FLUTTER_HOME && \
    git config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*" && \
    flutter doctor -v && \
    flutter config --no-analytics && \
    # Only precache for Android to save space
    flutter precache --android --no-ios --no-web --no-linux --no-windows --no-macos && \
    # Clean up Flutter cache and unnecessary files (but keep essential build files)
    rm -rf $FLUTTER_HOME/bin/cache/artifacts/ios* \
    && rm -rf $FLUTTER_HOME/bin/cache/artifacts/macos* \
    && rm -rf $FLUTTER_HOME/bin/cache/artifacts/linux* \
    && rm -rf $FLUTTER_HOME/bin/cache/artifacts/windows* \
    && rm -rf $FLUTTER_HOME/bin/cache/artifacts/fuchsia* \
    && rm -rf $FLUTTER_HOME/examples \
    && rm -rf $FLUTTER_HOME/dev/benchmarks
    # Keep .git, gradle scripts (CMakeLists.txt), and essential build files

# Configure Gradle for better performance
ENV GRADLE_USER_HOME=/opt/gradle
RUN mkdir -p $GRADLE_USER_HOME && \
    echo "org.gradle.daemon=true" >> $GRADLE_USER_HOME/gradle.properties && \
    echo "org.gradle.parallel=true" >> $GRADLE_USER_HOME/gradle.properties && \
    echo "org.gradle.caching=true" >> $GRADLE_USER_HOME/gradle.properties && \
    echo "org.gradle.jvmargs=-Xmx4g -XX:MaxMetaspaceSize=512m -XX:+HeapDumpOnOutOfMemoryError" >> $GRADLE_USER_HOME/gradle.properties

# Final cleanup to reduce image size
RUN apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /tmp/* \
    && rm -rf /var/cache/* \
    && rm -rf /root/.cache \
    && journalctl --vacuum-time=1s 2>/dev/null || true

# Create non-root user
RUN groupadd -r flutter -g 1000 && \
    useradd -r -u 1000 -g flutter -m -s /bin/bash flutter && \
    chown -R flutter:flutter $FLUTTER_HOME $ANDROID_HOME $GRADLE_USER_HOME

# Switch to non-root user for FVM installation
USER flutter

# Install FVM (Flutter Version Management) as non-root user
RUN curl -fsSL https://fvm.app/install.sh | bash || true
ENV PATH="/home/flutter/.pub-cache/bin:$PATH"

# Switch back to configure workspace
USER root

# Create work directory
WORKDIR /workspace
RUN chown flutter:flutter /workspace

# Copy build script
COPY build/build.sh /usr/local/bin/build.sh
RUN chmod +x /usr/local/bin/build.sh

# Switch to non-root user
USER flutter

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/build.sh"]
