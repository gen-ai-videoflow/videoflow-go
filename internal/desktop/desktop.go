// Package desktop serves the browser interface and API from one local process.
package desktop

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"6eek-go/internal/config"
)

// Configure keeps projects outside the download folder so updates preserve data.
func Configure(cfg *config.Config, uiDir string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	folder := filepath.Dir(executable)
	if uiDir == "" {
		uiDir = filepath.Join(folder, "frontend")
	}
	if _, err := os.Stat(filepath.Join(uiDir, "index.html")); err != nil {
		return "", fmt.Errorf("interface missing: extract the entire 6eek download before starting: %w", err)
	}
	dataRoot, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dataRoot = filepath.Join(dataRoot, "6eek")
	if err := os.MkdirAll(dataRoot, 0700); err != nil {
		return "", err
	}
	if os.Getenv("DATABASE_PATH") == "" {
		cfg.DatabasePath = filepath.Join(dataRoot, "db.sqlite")
	}
	if os.Getenv("STORAGE_ROOT") == "" {
		cfg.StorageRoot = filepath.Join(dataRoot, "storage")
	}
	// The bundled FFmpeg is preferred over the system installation.
	if err := os.Setenv("PATH", folder+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		return "", err
	}
	return uiDir, nil
}

func Handler(api http.Handler, uiDir, address string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", api))
	mux.Handle("/storage/", api)
	files := http.FileServer(http.Dir(uiDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := filepath.Join(uiDir, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		// Missing scripts/images must not receive HTML; application routes use SPA fallback.
		if strings.HasPrefix(r.URL.Path, "/_app/") || filepath.Ext(r.URL.Path) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(uiDir, "index.html"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The desktop app holds a paid API credential. CORS alone does not stop
		// cross-site writes, and a fixed Host prevents DNS rebinding to loopback.
		if r.Host != address {
			http.Error(w, "unexpected host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+address {
			http.Error(w, "foreign origin", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Run()
}
