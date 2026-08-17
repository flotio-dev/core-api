// Command swag-patch rebuilds the pinned swag CLI (v1.16.6) with a
// one-character upstream fix: swag v1.16.6's routerPattern regex rejects '@'
// in router paths, which the Flotio Core API needs for /auth/@me (contract
// Table 2-1 rows 7/8). Later swag releases adopted the same fix.
//
// Usage (from core-api/):
//
//	go run ./tools/swag-patch [version]
//
// It copies the (read-only) module-cache source of github.com/swaggo/swag@<version>
// into a writable workspace dir under $GOCACHE, applies the regex fix, and
// rebuilds the CLI where `go install` would have put it (GOBIN or GOPATH/bin),
// named swag (swag.exe on Windows). Run `go install
// github.com/swaggo/swag/cmd/swag@v1.16.6` first so the source is cached.
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	oldPattern = `[\w./\-{}\(\)+:$~]`
	newPattern = `[\w./\-{}\(\)+:$~@]`
)

func main() {
	version := "v1.16.6"
	if len(os.Args) > 1 {
		version = os.Args[1]
	}

	gomodcache := os.Getenv("GOMODCACHE")
	if gomodcache == "" {
		gopath := strings.TrimSpace(string(goEnv("GOPATH")))
		gomodcache = filepath.Join(gopath, "pkg", "mod")
	}

	srcDir := filepath.Join(gomodcache, "github.com", "swaggo", "swag@"+version)
	opFile := filepath.Join(srcDir, "operation.go")
	if _, err := os.Stat(opFile); err != nil {
		fatal("swag source not found at %s — run 'go install github.com/swaggo/swag/cmd/swag@%s' first: %v",
			srcDir, version, err)
	}

	// Copy the (read-only) module-cache source to a writable workspace dir.
	workDir := filepath.Join(strings.TrimSpace(string(goEnv("GOCACHE"))), "swag-src-"+version)
	if err := os.RemoveAll(workDir); err != nil {
		fatal("remove %s: %v", workDir, err)
	}
	if err := copyDir(srcDir, workDir); err != nil {
		fatal("copy %s to %s: %v", srcDir, workDir, err)
	}

	// Apply the routerPattern fix (idempotent).
	op := filepath.Join(workDir, "operation.go")
	content, err := os.ReadFile(op)
	if err != nil {
		fatal("read %s: %v", op, err)
	}
	if !strings.Contains(string(content), oldPattern) {
		fatal("routerPattern pattern %q not found in %s — swag source changed?", oldPattern, op)
	}
	patched := strings.ReplaceAll(string(content), oldPattern, newPattern)
	if err := os.WriteFile(op, []byte(patched), 0o644); err != nil {
		fatal("write %s: %v", op, err)
	}

	// Rebuild the patched CLI into GOBIN (or GOPATH/bin), like 'go install'.
	outBin := filepath.Join(goBin(), "swag"+exeSuffix())
	cmd := exec.Command("go", "build", "-o", outBin, "./cmd/swag")
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("go build patched swag: %v", err)
	}
	fmt.Printf("swag %s (patched for '@' in router paths) installed to %s\n", version, outBin)
}

func goEnv(key string) []byte {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		fatal("go env %s: %v", key, err)
	}
	return out
}

func goBin() string {
	if gb := strings.TrimSpace(string(goEnv("GOBIN"))); gb != "" {
		return gb
	}
	return filepath.Join(strings.TrimSpace(string(goEnv("GOPATH"))), "bin")
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return nil
	})
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "swag-patch: "+format+"\n", args...)
	os.Exit(1)
}
