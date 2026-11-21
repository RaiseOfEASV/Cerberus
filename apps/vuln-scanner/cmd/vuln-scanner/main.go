// Command vuln-scanner is the nightly Trivy scan behind Cerberoast's
// automatic findings. Run as a CronJob in an app namespace, it:
//
//  1. lists the images the namespace's pods are running (Kubernetes API,
//     with the pod's own ServiceAccount: get/list pods in that namespace);
//  2. scans each of our images (IMAGE_PREFIX) with Trivy;
//  3. posts each report to vuln-api's internal endpoint, which creates
//     findings for new vulnerabilities and closes the ones that are gone.
//
// An image whose scan or upload fails is skipped, so a broken scan never
// closes findings. The exit code is non-zero if anything failed, which marks
// the Job failed.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type config struct {
	Namespace    string
	VulnAPIURL   string
	Token        string
	ImagePrefix  string
	TrivyBin     string
	CacheDir     string
	KubeAPI      string
	SADir        string
	ScanTimeout  time.Duration
	TrivyOptions []string
}

func loadConfig(getenv func(string) string) (config, error) {
	or := func(k, def string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return def
	}
	c := config{
		Namespace:   strings.TrimSpace(getenv("NAMESPACE")),
		VulnAPIURL:  strings.TrimRight(strings.TrimSpace(getenv("VULN_API_URL")), "/"),
		Token:       strings.TrimSpace(getenv("SCANNER_TOKEN")),
		ImagePrefix: or("IMAGE_PREFIX", "ghcr.io/raiseofeasv/"),
		TrivyBin:    or("TRIVY_BIN", "/trivy"),
		CacheDir:    or("TRIVY_CACHE_DIR", "/tmp/trivy-cache"),
		KubeAPI:     or("KUBE_API", "https://kubernetes.default.svc"),
		SADir:       or("SA_DIR", "/var/run/secrets/kubernetes.io/serviceaccount"),
	}
	d, err := time.ParseDuration(or("SCAN_TIMEOUT", "10m"))
	if err != nil {
		return c, fmt.Errorf("SCAN_TIMEOUT: %w", err)
	}
	c.ScanTimeout = d
	if c.Namespace == "" {
		if b, err := os.ReadFile(filepath.Join(c.SADir, "namespace")); err == nil {
			c.Namespace = strings.TrimSpace(string(b))
		}
	}
	var missing []string
	for k, v := range map[string]string{"NAMESPACE": c.Namespace, "VULN_API_URL": c.VulnAPIURL, "SCANNER_TOKEN": c.Token} {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return c, fmt.Errorf("missing required settings: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

// podList is the part of a Kubernetes PodList we need.
type podList struct {
	Items []struct {
		Status struct {
			InitContainerStatuses []struct {
				ImageID string `json:"imageID"`
			} `json:"initContainerStatuses"`
			ContainerStatuses []struct {
				ImageID string `json:"imageID"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

// imagesFromPods returns the distinct digest-pinned images under prefix,
// sorted. imageID is what the node actually pulled ("repo@sha256:..."),
// sometimes with a "docker-pullable://" style scheme in front.
func imagesFromPods(body []byte, prefix string) ([]string, error) {
	var pl podList
	if err := json.Unmarshal(body, &pl); err != nil {
		return nil, fmt.Errorf("pod list: %w", err)
	}
	seen := map[string]bool{}
	add := func(id string) {
		if i := strings.Index(id, "://"); i >= 0 {
			id = id[i+3:]
		}
		if strings.HasPrefix(id, prefix) && strings.Contains(id, "@sha256:") {
			seen[id] = true
		}
	}
	for _, p := range pl.Items {
		for _, c := range p.Status.InitContainerStatuses {
			add(c.ImageID)
		}
		for _, c := range p.Status.ContainerStatuses {
			add(c.ImageID)
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// assetOf is the finding asset for an image: the repository without the
// digest, so findings carry over from one build of an app to the next.
func assetOf(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		return ref[:i]
	}
	return ref
}

func kubeClient(saDir string) (*http.Client, string, error) {
	ca, err := os.ReadFile(filepath.Join(saDir, "ca.crt"))
	if err != nil {
		return nil, "", fmt.Errorf("kubernetes CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, "", errors.New("kubernetes CA: no certificates")
	}
	tok, err := os.ReadFile(filepath.Join(saDir, "token"))
	if err != nil {
		return nil, "", fmt.Errorf("service account token: %w", err)
	}
	c := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	return c, strings.TrimSpace(string(tok)), nil
}

func listImages(ctx context.Context, c config, hc *http.Client, token string) ([]string, error) {
	u := c.KubeAPI + "/api/v1/namespaces/" + url.PathEscape(c.Namespace) + "/pods"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list pods: %s", resp.Status)
	}
	return imagesFromPods(body, c.ImagePrefix)
}

// runner runs Trivy; a variable so tests can fake it.
var runner = func(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

func updateDB(ctx context.Context, c config) error {
	_, err := runner(ctx, c.TrivyBin, "image", "--download-db-only", "--quiet", "--cache-dir", c.CacheDir)
	return err
}

func scan(ctx context.Context, c config, ref string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.ScanTimeout)
	defer cancel()
	args := []string{"image", "--quiet", "--format", "json", "--scanners", "vuln",
		"--skip-db-update", "--skip-java-db-update", "--image-src", "remote",
		"--cache-dir", c.CacheDir, "--timeout", c.ScanTimeout.String()}
	args = append(args, c.TrivyOptions...)
	return runner(ctx, c.TrivyBin, append(args, ref)...)
}

type result struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
	Closed  int `json:"closed"`
}

func upload(ctx context.Context, c config, hc *http.Client, asset string, report []byte) (result, error) {
	u := c.VulnAPIURL + "/internal/scans/trivy?asset=" + url.QueryEscape(asset)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(report)))
	if err != nil {
		return result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return result{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return result{}, fmt.Errorf("vuln-api: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var r result
	if err := json.Unmarshal(body, &r); err != nil {
		return result{}, fmt.Errorf("vuln-api answer: %w", err)
	}
	return r, nil
}

// scanAll scans and uploads each image; it returns how many failed.
func scanAll(ctx context.Context, log *slog.Logger, c config, api *http.Client, images []string) int {
	failed := 0
	for _, ref := range images {
		asset := assetOf(ref)
		report, err := scan(ctx, c, ref)
		if err != nil {
			log.Error("scan failed; its findings are left as they are", "image", ref, "err", err)
			failed++
			continue
		}
		r, err := upload(ctx, c, api, asset, report)
		if err != nil {
			log.Error("upload failed", "image", ref, "err", err)
			failed++
			continue
		}
		log.Info("scanned", "asset", asset, "image", ref, "created", r.Created, "already_open", r.Skipped, "closed", r.Closed)
	}
	return failed
}

func run(ctx context.Context, log *slog.Logger, getenv func(string) string) error {
	c, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	kc, token, err := kubeClient(c.SADir)
	if err != nil {
		return err
	}
	images, err := listImages(ctx, c, kc, token)
	if err != nil {
		return err
	}
	log.Info("images to scan", "namespace", c.Namespace, "count", len(images))
	if len(images) == 0 {
		return nil
	}
	if err := updateDB(ctx, c); err != nil {
		return fmt.Errorf("trivy database update: %w", err)
	}
	api := &http.Client{Timeout: 2 * time.Minute}
	if n := scanAll(ctx, log, c, api, images); n > 0 {
		return fmt.Errorf("%d of %d images failed", n, len(images))
	}
	return nil
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(context.Background(), log, os.Getenv); err != nil {
		log.Error("vuln-scanner", "err", err)
		os.Exit(1)
	}
}
