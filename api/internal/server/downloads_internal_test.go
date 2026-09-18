package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

// Fixture assets with digests pinned as literals, so the JSON golden below is
// a real byte-for-byte lock on the /version.json payload the CLI self-update
// parses (cli/internal/cli/update.go) rather than a restatement of the handler.
const (
	fixtureDarwinARM64       = "darwin-arm64 cli binary"
	fixtureDarwinARM64SHA256 = "69f5f9227b64cbd9c18d69f858a4b53b92d4ea05bdd2acf223478195b8ffc4ac"
	fixtureDarwinAMD64       = "darwin-amd64 cli binary"
	fixtureDarwinAMD64SHA256 = "4ff17b9b265aea869c7aaa91f959c8be0387e0daa44fd9ef1c788dc70b00b5b0"
	fixtureLinuxARM64        = "linux-arm64 cli binary"
	fixtureLinuxARM64SHA256  = "dc7508c1c017aefcd1ecfaf8957e296e5ecc7e302d8b10fb9c7f05cd69e3d836"
	fixtureLinuxAMD64        = "linux-amd64 cli binary"
	fixtureLinuxAMD64SHA256  = "efdce87302962ca205b206636d5514ceef7314e1539bd02408e9cbfc6f493b35"
)

// fixtureVersionJSON is what /version.json returns for the four fixture
// assets. Field names, field order, and the omitempty behaviour of sha256/size
// are part of the published contract.
const fixtureVersionJSON = `{"version":"dev","commit":"","install":"/install.sh","assets":[` +
	`{"os":"darwin","arch":"arm64","url":"/cli/darwin/arm64","sha256":"` + fixtureDarwinARM64SHA256 + `","size":23},` +
	`{"os":"darwin","arch":"amd64","url":"/cli/darwin/amd64","sha256":"` + fixtureDarwinAMD64SHA256 + `","size":23},` +
	`{"os":"linux","arch":"arm64","url":"/cli/linux/arm64","sha256":"` + fixtureLinuxARM64SHA256 + `","size":22},` +
	`{"os":"linux","arch":"amd64","url":"/cli/linux/amd64","sha256":"` + fixtureLinuxAMD64SHA256 + `","size":22}]}`

// fixtureCLIAssetFS is the four-platform stand-in for download_dist, which is
// empty in a dev checkout.
func fixtureCLIAssetFS() fstest.MapFS {
	return cliAssetMapFS(map[string][]byte{
		"aidocs-darwin-arm64": []byte(fixtureDarwinARM64),
		"aidocs-darwin-amd64": []byte(fixtureDarwinAMD64),
		"aidocs-linux-arm64":  []byte(fixtureLinuxARM64),
		"aidocs-linux-amd64":  []byte(fixtureLinuxAMD64),
	})
}

func cliAssetMapFS(assets map[string][]byte) fstest.MapFS {
	fsys := make(fstest.MapFS, len(assets))
	for name, data := range assets {
		fsys[cliAssetDir+"/"+name] = &fstest.MapFile{Data: data}
	}
	return fsys
}

// bigCLIAssetFS returns four assets of size bytes each, standing in for the
// stripped release binaries (15-25 MB apiece) whose per-request copies this
// package must not make.
func bigCLIAssetFS(size int) fstest.MapFS {
	assets := make(map[string][]byte, len(cliPlatforms))
	for i, p := range cliPlatforms {
		data := make([]byte, size)
		for j := range data {
			data[j] = byte(i) + byte(j)
		}
		assets[cliAssetName(p.os, p.arch)] = data
	}
	return cliAssetMapFS(assets)
}

// useCLIAssetFS points the download handlers at fsys for the duration of one
// test and restores the embedded filesystem afterwards.
func useCLIAssetFS(t *testing.T, fsys fs.FS) {
	t.Helper()
	previous := cliAssetFS
	cliAssetFS = fsys
	resetCLIAssetCache()
	t.Cleanup(func() {
		cliAssetFS = previous
		resetCLIAssetCache()
	})
}

// resetCLIAssetCache drops the process-wide asset metadata so each test sees
// its own stand-in assets from a cold start.
func resetCLIAssetCache() {
	cliAssetInfoOnce = sync.Once{}
	cliAssetInfos = nil
}

// newDownloadsTestRouter registers the download routes exactly as
// (*Server).routes does. These handlers touch no dependencies, so wiring them
// directly keeps the allocation measurements below free of database and
// middleware noise.
func newDownloadsTestRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	h := handlers{deps: dependencies{appOrigin: "https://aidocs.example"}}
	r.GET("/version.json", h.versionJSON)
	r.GET("/install.sh", h.installScript)
	r.GET("/cli/:os/:arch", h.downloadCLI)
	return r
}

func getDownload(t *testing.T, r http.Handler, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// discardWriter drops the response body. httptest.ResponseRecorder grows a
// buffer to hold whatever is written, which would swamp the handler's own
// allocations when the body is a multi-megabyte binary.
type discardWriter struct {
	header http.Header
	status int
	size   int
}

func newDiscardWriter() *discardWriter {
	return &discardWriter{header: make(http.Header), status: http.StatusOK}
}

func (w *discardWriter) Header() http.Header         { return w.header }
func (w *discardWriter) WriteHeader(status int)      { w.status = status }
func (w *discardWriter) Write(b []byte) (int, error) { w.size += len(b); return len(b), nil }

// allocBytesPerRequest reports the average bytes allocated while serving
// target, after a warm-up request so that one-time work (route matching
// caches, sync.Pool fills, and any legitimate first-request hashing) is not
// charged to the measured loop.
func allocBytesPerRequest(t *testing.T, r http.Handler, target string, requests uint64) uint64 {
	t.Helper()
	serve := func() *discardWriter {
		w := newDiscardWriter()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		return w
	}
	if got := serve().status; got != http.StatusOK {
		t.Fatalf("GET %s warm-up status = %d, want %d", target, got, http.StatusOK)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for range requests {
		if got := serve().status; got != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d", target, got, http.StatusOK)
		}
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / requests
}

// TestCLIAssetDirMatchesEmbed catches drift between the embed directive and
// the directory the handlers read from. A dev checkout builds no CLI binaries,
// so nothing else in the suite would notice a wrong path.
func TestCLIAssetDirMatchesEmbed(t *testing.T) {
	info, err := fs.Stat(downloadDist, cliAssetDir)
	if err != nil {
		t.Fatalf("stat %q in the embedded filesystem: %v", cliAssetDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory in the embedded filesystem", cliAssetDir)
	}
}

func TestVersionJSONReportsEmbeddedAssetDigests(t *testing.T) {
	useCLIAssetFS(t, fixtureCLIAssetFS())
	r := newDownloadsTestRouter()

	rr := getDownload(t, r, "/version.json", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != fixtureVersionJSON {
		t.Fatalf("version.json body\n got: %s\nwant: %s", got, fixtureVersionJSON)
	}
}

// TestVersionJSONIsStableAcrossRequests guards the cached metadata: a second
// request must produce the same digests and sizes as the first.
func TestVersionJSONIsStableAcrossRequests(t *testing.T) {
	useCLIAssetFS(t, fixtureCLIAssetFS())
	r := newDownloadsTestRouter()

	first := getDownload(t, r, "/version.json", nil).Body.String()
	second := getDownload(t, r, "/version.json", nil).Body.String()

	if first != second {
		t.Fatalf("version.json changed between requests\nfirst:  %s\nsecond: %s", first, second)
	}
	if first != fixtureVersionJSON {
		t.Fatalf("version.json body\n got: %s\nwant: %s", first, fixtureVersionJSON)
	}
}

// TestVersionJSONOmitsUnavailableAssets covers a build whose download_dist is
// partly empty: those platforms keep their URL but carry no sha256 or size.
func TestVersionJSONOmitsUnavailableAssets(t *testing.T) {
	useCLIAssetFS(t, cliAssetMapFS(map[string][]byte{
		"aidocs-darwin-arm64": []byte(fixtureDarwinARM64),
		"aidocs-linux-amd64":  {}, // present but empty, as a failed build leaves it
	}))
	r := newDownloadsTestRouter()

	rr := getDownload(t, r, "/version.json", nil)

	want := `{"version":"dev","commit":"","install":"/install.sh","assets":[` +
		`{"os":"darwin","arch":"arm64","url":"/cli/darwin/arm64","sha256":"` + fixtureDarwinARM64SHA256 + `","size":23},` +
		`{"os":"darwin","arch":"amd64","url":"/cli/darwin/amd64"},` +
		`{"os":"linux","arch":"arm64","url":"/cli/linux/arm64"},` +
		`{"os":"linux","arch":"amd64","url":"/cli/linux/amd64"}]}`
	if got := rr.Body.String(); got != want {
		t.Fatalf("version.json body\n got: %s\nwant: %s", got, want)
	}
}

// TestVersionJSONDoesNotCopyAssetsPerRequest is the regression test for the
// unauthenticated memory amplification: the endpoint used to read and hash
// every embedded binary on every call, so each request allocated the combined
// size of all four assets.
func TestVersionJSONDoesNotCopyAssetsPerRequest(t *testing.T) {
	const assetSize = 8 << 20 // 8 MiB each, 32 MiB per request if copied
	const budget = 256 << 10  // generous next to one 8 MiB asset
	useCLIAssetFS(t, bigCLIAssetFS(assetSize))
	r := newDownloadsTestRouter()

	perRequest := allocBytesPerRequest(t, r, "/version.json", 8)

	if perRequest > budget {
		t.Fatalf("GET /version.json allocated %d bytes per request, want at most %d (assets are %d bytes each)",
			perRequest, budget, assetSize)
	}
}

// TestVersionJSONConcurrentFirstRequests exercises the cache from a cold start
// under concurrency, which is how an unauthenticated endpoint is actually hit.
// Run with -race.
func TestVersionJSONConcurrentFirstRequests(t *testing.T) {
	useCLIAssetFS(t, fixtureCLIAssetFS())
	r := newDownloadsTestRouter()

	const concurrency = 16
	bodies := make([]string, concurrency)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/version.json", nil))
			bodies[i] = rr.Body.String()
		}()
	}
	close(start)
	wg.Wait()

	for i, body := range bodies {
		if body != fixtureVersionJSON {
			t.Fatalf("concurrent request %d body\n got: %s\nwant: %s", i, body, fixtureVersionJSON)
		}
	}
}

func TestDownloadCLIServesAssetBytes(t *testing.T) {
	useCLIAssetFS(t, fixtureCLIAssetFS())
	r := newDownloadsTestRouter()

	rr := getDownload(t, r, "/cli/linux/amd64", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != fixtureLinuxAMD64 {
		t.Fatalf("body = %q, want %q", got, fixtureLinuxAMD64)
	}
	if got, want := rr.Header().Get("Content-Disposition"), `attachment; filename="aidocs-linux-amd64"`; got != want {
		t.Fatalf("Content-Disposition = %q, want %q", got, want)
	}
	if got, want := rr.Header().Get("Content-Type"), "application/octet-stream"; got != want {
		t.Fatalf("Content-Type = %q, want %q", got, want)
	}
	if got, want := rr.Header().Get("ETag"), `"`+fixtureLinuxAMD64SHA256+`"`; got != want {
		t.Fatalf("ETag = %q, want %q", got, want)
	}
}

// TestDownloadCLIHonorsRangeRequests locks in the resumable-download support
// that serving the embedded file directly brings with it.
func TestDownloadCLIHonorsRangeRequests(t *testing.T) {
	useCLIAssetFS(t, fixtureCLIAssetFS())
	r := newDownloadsTestRouter()

	rr := getDownload(t, r, "/cli/linux/amd64", map[string]string{"Range": "bytes=6-10"})

	if rr.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusPartialContent)
	}
	if got, want := rr.Body.String(), fixtureLinuxAMD64[6:11]; got != want {
		t.Fatalf("ranged body = %q, want %q", got, want)
	}
	if got, want := rr.Header().Get("Content-Range"), "bytes 6-10/22"; got != want {
		t.Fatalf("Content-Range = %q, want %q", got, want)
	}
}

// TestDownloadCLIRevalidatesWithETag checks the strong validator the digest
// cache makes free: a CLI that already holds these bytes gets a 304.
func TestDownloadCLIRevalidatesWithETag(t *testing.T) {
	useCLIAssetFS(t, fixtureCLIAssetFS())
	r := newDownloadsTestRouter()

	rr := getDownload(t, r, "/cli/linux/amd64", map[string]string{"If-None-Match": `"` + fixtureLinuxAMD64SHA256 + `"`})

	if rr.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotModified)
	}
	if body := rr.Body.String(); body != "" {
		t.Fatalf("304 response carried a body: %q", body)
	}
}

// TestDownloadCLIDoesNotCopyAssetPerRequest covers the other embeddedCLIAsset
// caller: serving a download used to copy the whole binary into memory first.
func TestDownloadCLIDoesNotCopyAssetPerRequest(t *testing.T) {
	const assetSize = 8 << 20
	const budget = 1 << 20 // well under one asset, well over a streaming copy buffer
	useCLIAssetFS(t, bigCLIAssetFS(assetSize))
	r := newDownloadsTestRouter()

	perRequest := allocBytesPerRequest(t, r, "/cli/darwin/arm64", 8)

	if perRequest > budget {
		t.Fatalf("GET /cli/darwin/arm64 allocated %d bytes per request, want at most %d (asset is %d bytes)",
			perRequest, budget, assetSize)
	}
}

func TestDownloadCLIRejectsUnsupportedPlatform(t *testing.T) {
	useCLIAssetFS(t, fixtureCLIAssetFS())
	r := newDownloadsTestRouter()

	rr := getDownload(t, r, "/cli/plan9/riscv64", nil)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
	if got, want := rr.Body.String(), `{"error":{"code":"not_found","message":"unsupported platform"}}`; got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestDownloadCLIReportsMissingAsset(t *testing.T) {
	useCLIAssetFS(t, cliAssetMapFS(map[string][]byte{
		"aidocs-linux-arm64": {}, // present but empty
	}))
	r := newDownloadsTestRouter()

	for _, target := range []string{"/cli/linux/amd64", "/cli/linux/arm64"} {
		rr := getDownload(t, r, target, nil)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d", target, rr.Code, http.StatusNotFound)
		}
		want := `{"error":{"code":"not_found","message":"cli binary is not available in this build"}}`
		if got := rr.Body.String(); got != want {
			t.Fatalf("GET %s body = %s, want %s", target, got, want)
		}
	}
}

// TestInstallScriptIgnoresCLIAssets documents that the installer resolves
// binaries at run time over HTTP and embeds no digests, so caching them cannot
// change what /install.sh serves.
func TestInstallScriptIgnoresCLIAssets(t *testing.T) {
	r := newDownloadsTestRouter()

	useCLIAssetFS(t, fixtureCLIAssetFS())
	withAssets := getDownload(t, r, "/install.sh", nil)
	useCLIAssetFS(t, cliAssetMapFS(nil))
	withoutAssets := getDownload(t, r, "/install.sh", nil)

	if withAssets.Code != http.StatusOK || withoutAssets.Code != http.StatusOK {
		t.Fatalf("status = %d and %d, want %d", withAssets.Code, withoutAssets.Code, http.StatusOK)
	}
	if withAssets.Body.String() != withoutAssets.Body.String() {
		t.Fatal("install.sh changed with the set of embedded CLI assets")
	}
	if got := withAssets.Body.String(); !strings.Contains(got, `curl -fsSL "$url" -o "$bin_tmp"`) {
		t.Fatalf("install script does not download the CLI at run time: %s", got)
	}
}
