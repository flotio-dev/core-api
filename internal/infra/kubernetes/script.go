package kubernetes

import (
	"fmt"
	"strings"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
)

// versionBuildFlags returns the Flutter version flags to bake into the build.
// Empty version name and zero version code yield no flags (the build falls back
// to the values declared in pubspec.yaml).
func versionBuildFlags(versionName string, versionCode int64) string {
	flags := ""
	if versionName != "" {
		flags += " --build-name=" + versionName
	}
	if versionCode > 0 {
		flags += fmt.Sprintf(" --build-number=%d", versionCode)
	}
	return flags
}

// GenerateBuildRunnerScript generates a modular shell script based on ProjectConfig
func GenerateBuildRunnerScript(config BuildConfig, projectConfig *dbEngine.ProjectConfig) string {
	var sb strings.Builder

	// Header and setup
	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("set -e\n")
	sb.WriteString("set -o pipefail\n\n")

	sb.WriteString("RED='\\033[0;31m'\n")
	sb.WriteString("GREEN='\\033[0;32m'\n")
	sb.WriteString("YELLOW='\\033[1;33m'\n")
	sb.WriteString("NC='\\033[0m'\n\n")

	sb.WriteString("log_step() {\n")
	sb.WriteString("  echo -e \"${GREEN}===========================================${NC}\"\n")
	sb.WriteString("  echo -e \"${GREEN}STEP: $1${NC}\"\n")
	sb.WriteString("  echo -e \"${GREEN}===========================================${NC}\"\n")
	sb.WriteString("}\n\n")

	// Phase 1: Provisioning / Environment Info
	sb.WriteString("log_step \"Provisioning & Environment Information\"\n")
	sb.WriteString("echo \"- Java Version: $(java -version 2>&1 | head -n 1)\"\n")
	sb.WriteString("echo \"- Android SDK: $ANDROID_SDK_ROOT\"\n")
	if projectConfig != nil && projectConfig.FlutterVersion != "" {
		sb.WriteString(fmt.Sprintf("echo \"- Target Flutter Version: %s\"\n", projectConfig.FlutterVersion))
	}
	sb.WriteString("echo \"- OS Info: $(uname -a)\"\n")
	sb.WriteString("echo \"- Memory Info: $(free -h | grep Mem | awk '{print $2}')\"\n")
	sb.WriteString("echo \"- Environment variables injected from /env assets\"\n")

	// Phase 2: Clone
	sb.WriteString("log_step \"Cloning Repository\"\n")
	gitRepo := ""
	buildFolder := ""
	if projectConfig != nil {
		gitRepo = projectConfig.GitRepo
		buildFolder = projectConfig.ProjectPath
	}

	if config.GitUsername != "" && config.GitToken != "" {
		sb.WriteString(fmt.Sprintf("GIT_URL_WITH_AUTH=$(echo \"%s\" | sed \"s|https://|https://%s:%s@|\")\n", gitRepo, config.GitUsername, config.GitToken))
		sb.WriteString(fmt.Sprintf("git clone --depth 1 --branch \"%s\" \"$GIT_URL_WITH_AUTH\" /workspace/repo\n", config.GitBranch))
	} else {
		sb.WriteString(fmt.Sprintf("git clone --depth 1 --branch \"%s\" \"%s\" /workspace/repo\n", config.GitBranch, gitRepo))
	}
	
	if buildFolder != "" {
		sb.WriteString(fmt.Sprintf("cd \"/workspace/repo/%s\"\n", buildFolder))
	} else {
		sb.WriteString("cd /workspace/repo\n")
	}
	sb.WriteString("echo \"- Working directory: $(pwd)\"\n\n")

	// Phase 3: Post-Clone Script
	if projectConfig != nil && projectConfig.PostCloneScript != "" {
		sb.WriteString("log_step \"Running Post-Clone Script\"\n")
		sb.WriteString(projectConfig.PostCloneScript + "\n\n")
	}

	// Phase 4: Flutter Setup
	sb.WriteString("log_step \"Setting up Flutter\"\n")
	if projectConfig != nil && projectConfig.FlutterVersion != "" {
		// Download the specific Flutter SDK version directly from the official archive.
		// FVM is not available in the build image; we fetch and extract the SDK instead.
		version := projectConfig.FlutterVersion
		sb.WriteString(fmt.Sprintf(
			"FLUTTER_DIR=\"/opt/flutter-%s\"\n"+
				"if [ ! -x \"$FLUTTER_DIR/bin/flutter\" ]; then\n"+
				"  CHANNEL=\"${FLUTTER_CHANNEL:-stable}\"\n"+
				"  SDK_URL=\"https://storage.googleapis.com/flutter_infra_release/releases/${CHANNEL}/linux/flutter_linux_%s-${CHANNEL}.tar.xz\"\n"+
				"  echo \"  Downloading Flutter SDK from $SDK_URL\"\n"+
				"  if wget -q --timeout=300 \"$SDK_URL\" -O flutter.tar.xz 2>/dev/null; then\n"+
				"    tar -xf flutter.tar.xz -C /opt\n"+
				"    mv /opt/flutter \"$FLUTTER_DIR\"\n"+
				"    rm flutter.tar.xz\n"+
				"    echo \"  ✓ Flutter %s installed\"\n"+
				"  else\n"+
				"    echo \"  ⚠ Failed to download Flutter %s — falling back to system Flutter\"\n"+
				"  fi\n"+
				"fi\n"+
				"if [ -x \"$FLUTTER_DIR/bin/flutter\" ]; then\n"+
				"  export PATH=\"$FLUTTER_DIR/bin:$PATH\"\n"+
				"fi\n",
			version, version, version, version))
	}
	sb.WriteString("flutter --version | head -n 1\n\n")

	// Phase 5: Cache Restore
	sb.WriteString("log_step \"Restoring Cache\"\n")
	sb.WriteString("restore_cache_if_enabled || echo \"- Cache restore failed, continuing...\"\n\n")

	// Phase 6: Env Files (External Configuration Files)
	sb.WriteString("log_step \"Processing Configuration Files (google-services.json, etc.)\"\n")
	sb.WriteString("if [ -d \"$ENV_FILES_DIR\" ] && [ \"$(ls -A $ENV_FILES_DIR)\" ]; then\n")
	sb.WriteString("  for file in \"$ENV_FILES_DIR\"/*; do\n")
	sb.WriteString("    if [ -f \"$file\" ]; then\n")
	sb.WriteString("      filename=$(basename \"$file\")\n")
	sb.WriteString("      if [[ $filename == *\"::\"* ]]; then\n")
	sb.WriteString("        dest_path=\"${filename#*::}\"\n")
	sb.WriteString("        dest_path=\"${dest_path//__//}\"\n")
	sb.WriteString("        mkdir -p \"$(dirname \"$dest_path\")\"\n")
	sb.WriteString("        cp \"$file\" \"$dest_path\"\n")
	sb.WriteString("        echo \"    ✓ $filename -> $dest_path\"\n")
	sb.WriteString("      else\n")
	sb.WriteString("        cp \"$file\" \"./$filename\"\n")
	sb.WriteString("        echo \"    ✓ $filename -> ./$filename\"\n")
	sb.WriteString("      fi\n")
	sb.WriteString("    fi\n")
	sb.WriteString("  done\n")
	sb.WriteString("else\n")
	sb.WriteString("  echo \"- No environment files found\"\n")
	sb.WriteString("fi\n\n")

	// Phase 7: Keystore Setup
	if config.Platform == "android" {
		sb.WriteString("log_step \"Setting up Android Keystore\"\n")
		sb.WriteString("if [ -f \"$KEYSTORE_PATH\" ]; then\n")
		sb.WriteString("  KEY_PROPERTIES_PATH=\"$(pwd)/android/key.properties\"\n")
		sb.WriteString("  mkdir -p \"$(dirname \"$KEY_PROPERTIES_PATH\")\"\n")
		sb.WriteString("  cat > \"$KEY_PROPERTIES_PATH\" << EOF\n")
		sb.WriteString("storePassword=${KEYSTORE_PASSWORD:-android}\n")
		sb.WriteString("keyPassword=${KEY_PASSWORD:-android}\n")
		sb.WriteString("keyAlias=${KEY_ALIAS:-key}\n")
		sb.WriteString("storeFile=$KEYSTORE_PATH\n")
		sb.WriteString("EOF\n")
		sb.WriteString("  echo \"  ✓ Keystore configured\"\n")
		sb.WriteString("else\n")
		sb.WriteString("  echo \"  • No keystore provided\"\n")
		sb.WriteString("fi\n\n")
	}

	// Phase 8: Dependencies
	sb.WriteString("log_step \"Fetching Dependencies\"\n")
	sb.WriteString("flutter pub get\n\n")

	// Phase 9: Pre-Test Script
	if projectConfig != nil && projectConfig.PreTestScript != "" {
		sb.WriteString("log_step \"Running Pre-Test Script\"\n")
		sb.WriteString(projectConfig.PreTestScript + "\n\n")
	}

	// Phase 10: Testing & Analysis
	if projectConfig != nil {
		if projectConfig.EnableFlutterAnalyze {
			sb.WriteString("log_step \"Running Flutter Analysis\"\n")
			args := projectConfig.FlutterAnalyzeArgs
			if args == "" {
				args = "analyze"
			}
			sb.WriteString(fmt.Sprintf("flutter %s\n\n", args))
		}

		if projectConfig.EnableFlutterTest {
			sb.WriteString("log_step \"Running Flutter Tests\"\n")
			args := projectConfig.FlutterTestArgs
			if args == "" {
				args = "test"
			}
			sb.WriteString(fmt.Sprintf("flutter %s\n\n", args))
		}
	}

	// Phase 11: Post-Test Script
	if projectConfig != nil && projectConfig.PostTestScript != "" {
		sb.WriteString("log_step \"Running Post-Test Script\"\n")
		sb.WriteString(projectConfig.PostTestScript + "\n\n")
	}

	// Phase 12: Pre-Build Script
	if projectConfig != nil && projectConfig.PreBuildScript != "" {
		sb.WriteString("log_step \"Running Pre-Build Script\"\n")
		sb.WriteString(projectConfig.PreBuildScript + "\n\n")
	}

	// Phase 13: Build
	sb.WriteString("log_step \"Building Application\"\n")
	buildCmd := ""
	mode := "release"
	if projectConfig != nil && projectConfig.BuildMode != "" {
		mode = projectConfig.BuildMode
	} else if config.BuildMode != "" {
		mode = config.BuildMode
	}

	switch config.Platform {
	case "android":
		target := "apk"
		if projectConfig != nil && projectConfig.AndroidBuildFormat != "" {
			target = projectConfig.AndroidBuildFormat
		} else if config.BuildTarget != "" {
			target = config.BuildTarget
		}
		
		if target == "aab" || target == "appbundle" {
			buildCmd = fmt.Sprintf("flutter build appbundle --%s", mode)
		} else {
			buildCmd = fmt.Sprintf("flutter build apk --%s", mode)
		}

		buildCmd += versionBuildFlags(config.VersionName, config.VersionCode)

		if projectConfig != nil && projectConfig.AndroidBuildArgs != "" {
			buildCmd += " " + projectConfig.AndroidBuildArgs
		}
	case "web":
		buildCmd = fmt.Sprintf("flutter build web --%s", mode)
		if projectConfig != nil && projectConfig.WebBuildArgs != "" {
			buildCmd += " " + projectConfig.WebBuildArgs
		}
	case "linux":
		buildCmd = fmt.Sprintf("flutter build linux --%s", mode)
	}

	if buildCmd != "" {
		sb.WriteString(buildCmd + "\n\n")
	}

	// Phase 14: Post-Build Script
	if projectConfig != nil && projectConfig.PostBuildScript != "" {
		sb.WriteString("log_step \"Running Post-Build Script\"\n")
		sb.WriteString(projectConfig.PostBuildScript + "\n\n")
	}

	// Phase 15: Artifact Collection
	sb.WriteString("log_step \"Collecting Artifacts\"\n")
	sb.WriteString("mkdir -p \"$OUTPUT_DIR\"\n")
	if config.Platform == "android" {
		sb.WriteString("APK_FILE=$(find build/app/outputs/flutter-apk -name \"*.apk\" | head -n 1)\n")
		sb.WriteString("AAB_FILE=$(find build/app/outputs/bundle -name \"*.aab\" | head -n 1)\n")
		sb.WriteString("if [ -f \"$APK_FILE\" ]; then cp \"$APK_FILE\" \"$OUTPUT_DIR/app-${BUILD_ID}.apk\"; fi\n")
		sb.WriteString("if [ -f \"$AAB_FILE\" ]; then cp \"$AAB_FILE\" \"$OUTPUT_DIR/app-${BUILD_ID}.aab\"; fi\n")
	} else if config.Platform == "web" {
		sb.WriteString("tar -czf \"$OUTPUT_DIR/web-build-${BUILD_ID}.tar.gz\" -C build/web .\n")
	}
	sb.WriteString("\n")

	// Phase 16: Pre-Publish Script
	if projectConfig != nil && projectConfig.PrePublishScript != "" {
		sb.WriteString("log_step \"Running Pre-Publish Script\"\n")
		sb.WriteString(projectConfig.PrePublishScript + "\n\n")
	}

	// Phase 17: Upload
	sb.WriteString("log_step \"Uploading Results\"\n")
	sb.WriteString("if [ -n \"$AWS_S3_BUCKET\" ]; then\n")
	sb.WriteString("  S3_PATH=\"s3://${AWS_S3_BUCKET}/${AWS_S3_PREFIX}/${BUILD_ID}\"\n")
	sb.WriteString("  ENDPOINT_ARG=\"\"\n")
	sb.WriteString("  [ -n \"$AWS_S3_ENDPOINT\" ] && ENDPOINT_ARG=\"--endpoint-url $AWS_S3_ENDPOINT\"\n")
	sb.WriteString("  aws s3 cp \"$OUTPUT_DIR\" \"$S3_PATH\" --recursive --region \"$AWS_REGION\" $ENDPOINT_ARG\n")
	sb.WriteString("fi\n\n")

	// Phase 18: Cache Save
	sb.WriteString("log_step \"Saving Cache\"\n")
	sb.WriteString("save_cache_if_enabled || echo \"- Cache upload failed\"\n\n")

	sb.WriteString("log_step \"Build Completed Successfully\"\n")

	return sb.String()
}
