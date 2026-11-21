package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const pods = `{"items":[
 {"status":{"initContainerStatuses":[{"imageID":"ghcr.io/raiseofeasv/vuln-api@sha256:aaa"}],
            "containerStatuses":[{"imageID":"ghcr.io/raiseofeasv/vuln-api@sha256:aaa"}]}},
 {"status":{"containerStatuses":[{"imageID":"docker-pullable://ghcr.io/raiseofeasv/vuln-web@sha256:bbb"}]}},
 {"status":{"containerStatuses":[{"imageID":"docker.io/library/postgres@sha256:ccc"}]}},
 {"status":{"containerStatuses":[{"imageID":""}]}}
]}`

func TestImagesFromPods(t *testing.T) {
	got, err := imagesFromPods([]byte(pods), "ghcr.io/raiseofeasv/")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ghcr.io/raiseofeasv/vuln-api@sha256:aaa", "ghcr.io/raiseofeasv/vuln-web@sha256:bbb"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if assetOf(want[0]) != "ghcr.io/raiseofeasv/vuln-api" {
		t.Fatalf("asset: %q", assetOf(want[0]))
	}
}

func TestLoadConfigNeedsTheEssentials(t *testing.T) {
	_, err := loadConfig(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "NAMESPACE, SCANNER_TOKEN, VULN_API_URL") {
		t.Fatalf("got %v", err)
	}
}

func TestScanAllUploadsAndSkipsFailures(t *testing.T) {
	var posted []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/scans/trivy" || r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		b, _ := io.ReadAll(r.Body)
		posted = append(posted, r.URL.Query().Get("asset")+" "+string(b))
		_, _ = w.Write([]byte(`{"created":1,"skipped":0,"closed":2}`))
	}))
	defer api.Close()

	old := runner
	defer func() { runner = old }()
	runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		ref := args[len(args)-1]
		if strings.Contains(ref, "vuln-web") {
			return nil, errors.New("registry unreachable")
		}
		return []byte(`{"SchemaVersion":2}`), nil
	}

	c := config{VulnAPIURL: api.URL, Token: "tok", TrivyBin: "trivy", ScanTimeout: 1e9}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	failed := scanAll(context.Background(), log, c, api.Client(),
		[]string{"ghcr.io/raiseofeasv/vuln-api@sha256:aaa", "ghcr.io/raiseofeasv/vuln-web@sha256:bbb"})
	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	// The failed image was never uploaded, so none of its findings get closed.
	if len(posted) != 1 || posted[0] != `ghcr.io/raiseofeasv/vuln-api {"SchemaVersion":2}` {
		t.Fatalf("posted %q", posted)
	}
}
