# Flutter Build Container - Production Ready
# Includes Android SDK, Java, Flutter and all necessary build tools

FROM ubuntu:22.04

# Avoid prompts from apt
ENV DEBIAN_FRONTEND=noninteractive

# Define versions
ENV FLUTTER_VERSION=stable
ENV ANDROID_SDK_VERSION=9477386
ENV ANDROID_BUILD_TOOLS_VERSION=34.0.0
ENV ANDROID_PLATFORMS_VERSION=34
ENV JAVA_VERSION=17

# Install base dependencies
RUN apt-get update && apt-get install -y \
    curl \
    git \
    unzip \
    xz-utils \
    zip \
    libglu1-mesa \
    openjdk-${JAVA_VERSION}-jdk \
    wget \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Set Java environment
ENV JAVA_HOME=/usr/lib/jvm/java-${JAVA_VERSION}-openjdk-amd64
ENV PATH=$PATH:$JAVA_HOME/bin

# Install Android SDK
ENV ANDROID_HOME=/opt/android-sdk
ENV ANDROID_SDK_ROOT=$ANDROID_HOME
ENV PATH=$PATH:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$ANDROID_HOME/build-tools/${ANDROID_BUILD_TOOLS_VERSION}

RUN mkdir -p $ANDROID_HOME/cmdline-tools && \
    cd $ANDROID_HOME/cmdline-tools && \
    wget -q https://dl.google.com/android/repository/commandlinetools-linux-${ANDROID_SDK_VERSION}_latest.zip && \
    unzip commandlinetools-linux-${ANDROID_SDK_VERSION}_latest.zip && \
    rm commandlinetools-linux-${ANDROID_SDK_VERSION}_latest.zip && \
    mv cmdline-tools latest

# Accept licenses and install Android components
RUN yes | sdkmanager --licenses && \
    sdkmanager "platform-tools" \
    "platforms;android-${ANDROID_PLATFORMS_VERSION}" \
    "build-tools;${ANDROID_BUILD_TOOLS_VERSION}" \
    "extras;google;google_play_services" \
    "extras;google;m2repository" \
    "extras;android;m2repository" \
    "ndk;25.1.8937393"

# Install Flutter
ENV FLUTTER_HOME=/opt/flutter
ENV PATH=$PATH:$FLUTTER_HOME/bin

RUN git clone https://github.com/flutter/flutter.git -b ${FLUTTER_VERSION} $FLUTTER_HOME && \
    flutter doctor -v && \
    flutter config --no-analytics && \
    flutter precache

# Create work directory
WORKDIR /workspace

# Copy build script
COPY build/build.sh /usr/local/bin/build.sh
RUN chmod +x /usr/local/bin/build.sh

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/build.sh"]
