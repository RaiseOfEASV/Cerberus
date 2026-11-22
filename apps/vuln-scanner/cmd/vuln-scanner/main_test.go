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
		return []byte(`{"SchemaVersion":2,"Results":[]}`), nil
	}

	c := config{VulnAPIURL: api.URL, Token: "tok", TrivyBin: "trivy", ScanTimeout: 1e9}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	failed := scanAll(context.Background(), log, c, api.Client(),
		targets([]string{"ghcr.io/raiseofeasv/vuln-api@sha256:aaa", "ghcr.io/raiseofeasv/vuln-web@sha256:bbb"}, nil))
	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	// The failed image was never uploaded, so none of its findings get closed.
	if len(posted) != 1 || posted[0] != `ghcr.io/raiseofeasv/vuln-api {"SchemaVersion":2,"ArtifactName":"","Results":[]}` {
		t.Fatalf("posted %q", posted)
	}
}

func TestCatalogImagesAreAddedOnce(t *testing.T) {
	cat, err := readImagesFile("# generated\npostgres postgres:16-alpine@sha256:ccc\n\nhashicorp/vault hashicorp/vault:1.17@sha256:ddd\n")
	if err != nil {
		t.Fatal(err)
	}
	// vuln-api is both running and (hypothetically) listed: scanned once, as the running image.
	cat = append(cat, target{Asset: "other", Ref: "ghcr.io/raiseofeasv/vuln-api@sha256:aaa"})
	got := targets([]string{"ghcr.io/raiseofeasv/vuln-api@sha256:aaa"}, cat)
	want := []target{
		{"ghcr.io/raiseofeasv/vuln-api", "ghcr.io/raiseofeasv/vuln-api@sha256:aaa"},
		{"postgres", "postgres:16-alpine@sha256:ccc"},
		{"hashicorp/vault", "hashicorp/vault:1.17@sha256:ddd"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("target %d: %v, want %v", i, got[i], want[i])
		}
	}
}

func TestImagesFileRejectsUnpinnedOrMalformed(t *testing.T) {
	for _, bad := range []string{"postgres postgres:16-alpine", "just-one-field", "a b c@sha256:x"} {
		if _, err := readImagesFile(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestCompactKeepsWhatVulnAPIReads(t *testing.T) {
	in := `{"SchemaVersion":2,"ArtifactName":"img","Results":[{"Target":"t","Class":"os-pkgs","Vulnerabilities":[
	  {"VulnerabilityID":"CVE-2026-1234","PkgName":"zlib","InstalledVersion":"1","FixedVersion":"2","Severity":"HIGH",
	   "Title":"T","Description":"D","PrimaryURL":"u","References":["r1","r2"],"Layer":{"Digest":"x"},
	   "CVSS":{"nvd":{"V3Score":7.5,"V3Vector":"AV:N"},"redhat":{"V3Score":5}}}]}]}`
	out, err := compact([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"VulnerabilityID":"CVE-2026-1234"`, `"PkgName":"zlib"`, `"FixedVersion":"2"`, `"Severity":"HIGH"`, `"Description":"D"`, `"PrimaryURL":"u"`, `"CVSS":{"nvd":{"V3Score":7.5}}`, `"ArtifactName":"img"`, `"Target":"t"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	for _, gone := range []string{"References", "Layer", "redhat", "V3Vector", "Class"} {
		if strings.Contains(s, gone) {
			t.Errorf("%s should be dropped: %s", gone, s)
		}
	}
	if _, err := compact([]byte("not json")); err == nil {
		t.Error("accepted a non-JSON report")
	}
}

func TestUpdateDBFetchesTheJavaDBToo(t *testing.T) {
	old := runner
	defer func() { runner = old }()
	var calls []string
	runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return nil, nil
	}
	if err := updateDB(context.Background(), config{TrivyBin: "trivy", CacheDir: "/c"}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || !strings.Contains(calls[0], "--download-db-only") || !strings.Contains(calls[1], "--download-java-db-only") {
		t.Fatalf("calls: %q", calls)
	}
}
