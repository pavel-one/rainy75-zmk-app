// Package update проверяет релизы на GitHub и ставит новую версию поверх
// запущенной: скачивает архив под свою ОС, подменяет приложение и
// перезапускает его.
package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"rainy75/internal/i18n"
)

const (
	Repo   = "pavel-one/rainy75-zmk-app"
	latest = "https://api.github.com/repos/" + Repo + "/releases/latest"
)

type Release struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	URL     string `json:"url"`
	Asset   string `json:"asset"`
	Size    int64  `json:"size"`
}

var client = &http.Client{Timeout: 30 * time.Second}

// AssetName — архив релиза для этой ОС (см. .github/workflows/release.yml).
func AssetName() string {
	switch runtime.GOOS {
	case "darwin":
		return "rainy75-macos-universal.zip"
	case "windows":
		return "rainy75-windows-amd64.zip"
	default:
		return "rainy75-linux-amd64.tar.gz"
	}
}

func Latest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latest, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, i18n.Errorf("нет связи с GitHub: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, i18n.Errorf("GitHub ответил %s", resp.Status)
	}
	var gh struct {
		Tag    string `json:"tag_name"`
		Body   string `json:"body"`
		URL    string `json:"html_url"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return Release{}, err
	}
	r := Release{Version: strings.TrimPrefix(gh.Tag, "v"), Notes: gh.Body, URL: gh.URL}
	for _, a := range gh.Assets {
		if a.Name == AssetName() {
			r.Asset, r.Size = a.URL, a.Size
		}
	}
	return r, nil
}

// Newer сообщает, что версия a новее b (обе в виде 1.2.3).
func Newer(a, b string) bool {
	pa, pb := parse(a), parse(b)
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) [3]int {
	var out [3]int
	v, _, _ = strings.Cut(strings.TrimPrefix(v, "v"), "-")
	for i, part := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(part)
	}
	return out
}

// Install скачивает архив релиза, ставит его на место запущенного приложения
// и запускает новую версию, как только эта завершится. Остаётся только выйти.
func Install(ctx context.Context, r Release, progress func(done, total int64)) error {
	if r.Asset == "" {
		return i18n.Errorf("в релизе %s нет сборки для этой ОС", r.Version)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "rainy75-update-")
	if err != nil {
		return err
	}
	archive := filepath.Join(tmp, AssetName())
	if err := download(ctx, r.Asset, archive, progress); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	dir := filepath.Join(tmp, "new")
	if err := extract(archive, dir); err != nil {
		os.RemoveAll(tmp)
		return i18n.Errorf("распаковка обновления: %v", err)
	}
	return replace(exe, dir)
}

func download(ctx context.Context, url, dst string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return i18n.Errorf("загрузка обновления: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return i18n.Errorf("загрузка обновления: %v", resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, &counter{r: resp.Body, total: resp.ContentLength, fn: progress}); err != nil {
		return i18n.Errorf("загрузка обновления: %v", err)
	}
	return f.Close()
}

type counter struct {
	r     io.Reader
	done  int64
	total int64
	fn    func(done, total int64)
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.done += int64(n)
	if c.fn != nil {
		c.fn(c.done, c.total)
	}
	return n, err
}

func extract(archive, dst string) error {
	if strings.HasSuffix(archive, ".tar.gz") {
		return untar(archive, dst)
	}
	return unzip(archive, dst)
}

// safePath не даёт файлам архива выйти за пределы dst.
func safePath(dst, name string) (string, error) {
	p := filepath.Join(dst, name)
	if !strings.HasPrefix(p, filepath.Clean(dst)+string(os.PathSeparator)) {
		return "", fmt.Errorf("bad path in archive: %s", name)
	}
	return p, nil
}

func unzip(archive, dst string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		p, err := safePath(dst, f.Name)
		if err != nil {
			return err
		}
		switch mode := f.Mode(); {
		case mode.IsDir():
			err = os.MkdirAll(p, 0o755)
		case mode&os.ModeSymlink != 0:
			err = unzipLink(f, p)
		default:
			err = unzipFile(f, p, mode.Perm())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func unzipLink(f *zip.File, p string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	target, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.Symlink(string(target), p)
}

func unzipFile(f *zip.File, p string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return writeFile(p, rc, perm)
}

func writeFile(p string, r io.Reader, perm os.FileMode) error {
	out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func untar(archive, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := safePath(dst, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(p, 0o755)
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
				err = writeFile(p, tr, os.FileMode(h.Mode).Perm())
			}
		}
		if err != nil {
			return err
		}
	}
}
