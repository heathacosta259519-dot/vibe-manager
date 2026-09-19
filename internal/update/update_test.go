package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.5.0", "0.5.0", 0},
		{"v0.5.0", "0.5.0", 0},
		{"0.5.1", "0.5.0", 1},
		{"0.5.0", "0.5.1", -1},
		{"0.6", "0.5.9", 1},
		{"0.5.0", "0.5", 0},
		{"1.0.0", "0.9.9", 1},
		{"0.10.0", "0.9.0", 1},
		{"0.5.1-rc1", "0.5.1", 0},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d，想要 %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCheckAssetURL(t *testing.T) {
	for _, ok := range []string{
		"https://github.com/o/r/releases/download/v1/x.exe",
		"https://objects.githubusercontent.com/x/y.exe",
	} {
		if err := checkAssetURL(ok); err != nil {
			t.Errorf("%s 应当放行：%v", ok, err)
		}
	}
	for _, bad := range []string{
		"http://github.com/o/r/x.exe",   // 不是 https
		"https://evil.example.com/x.exe", // 不是 GitHub 域名
		"https://github.com.evil.com/x",  // 后缀伪装
	} {
		if err := checkAssetURL(bad); err == nil {
			t.Errorf("%s 应当被拒绝", bad)
		}
	}
}

// withTestServer 把 apiBase 与允许的下载域名指到本地 TLS 测试服务器。
// 用 TLS 是为了让 checkAssetURL 的 https 要求也参与测试。
func withTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	// 注意用 Hostname()：checkAssetURL 比对的是不带端口的域名
	host := strings.Split(strings.TrimPrefix(srv.URL, "https://"), ":")[0]

	oldBase, oldHosts := apiBase, allowedHosts
	oldCheck, oldDownload := checkClient, downloadClient
	apiBase = srv.URL
	allowedHosts = map[string]bool{host: true}
	checkClient = srv.Client()
	downloadClient = srv.Client()

	t.Cleanup(func() {
		srv.Close()
		apiBase, allowedHosts = oldBase, oldHosts
		checkClient, downloadClient = oldCheck, oldDownload
	})
	return srv
}

func TestLatestFindsNewerRelease(t *testing.T) {
	payload := []byte("fake-exe-payload")
	sum := sha256.Sum256(payload)

	srvURL := ""
	srv := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + Repo + "/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v0.6.0","body":"修了几个问题","html_url":"https://github.com/o/r/releases/tag/v0.6.0",
				"assets":[{"name":"Vibe-Manager.exe","size":%d,"digest":"sha256:%s","browser_download_url":"%s/download.exe"}]}`,
				len(payload), hex.EncodeToString(sum[:]), srvURL)
		case "/download.exe":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	})
	srvURL = srv.URL

	rel, err := Latest("0.5.0")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel == nil {
		t.Fatal("应当发现新版本")
	}
	if rel.Version != "0.6.0" || rel.Size != int64(len(payload)) || rel.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("解析结果不对：%+v", rel)
	}
	if rel.Notes != "修了几个问题" || rel.PageURL == "" {
		t.Fatalf("备注或页面地址丢了：%+v", rel)
	}

	// 已是最新、或本地版本更高时，都不该报「有新版本」
	for _, cur := range []string{"0.6.0", "0.6.1", "v0.6.0"} {
		if rel, err := Latest(cur); err != nil || rel != nil {
			t.Fatalf("current=%s 应当没有更新，得到 %+v, %v", cur, rel, err)
		}
	}
}

func TestLatestWithoutAsset(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v0.6.0","assets":[{"name":"其他.zip","size":1,"browser_download_url":"https://github.com/x"}]}`)
	})
	if _, err := Latest("0.5.0"); err == nil {
		t.Fatal("没有附带 exe 的新版本应当报错，而不是给出一个下不了的更新")
	}
}

func TestLatestNoReleaseAtAll(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	rel, err := Latest("0.5.0")
	if err != nil || rel != nil {
		t.Fatalf("仓库还没有 release 时应当安静地当作没有更新，得到 %+v, %v", rel, err)
	}
}

func TestDownloadChecksDigestAndSize(t *testing.T) {
	payload := []byte("a-real-payload")
	sum := sha256.Sum256(payload)

	srv := withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})

	dir := t.TempDir()
	dest := filepath.Join(dir, "dl.exe")

	good := &Release{URL: srv.URL + "/x.exe", Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:])}
	if err := Download(good, dest); err != nil {
		t.Fatalf("正常下载失败：%v", err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != string(payload) {
		t.Fatalf("下载内容不对：%q, %v", got, err)
	}

	// 摘要不符：必须报错，并且不留下那个文件
	bad := *good
	bad.SHA256 = strings.Repeat("0", 64)
	if err := Download(&bad, dest); err == nil {
		t.Fatal("摘要不符应当报错")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("校验没过时应当把半成品删掉")
	}

	// 字节数不符同理
	short := *good
	short.Size = int64(len(payload)) + 10
	if err := Download(&short, dest); err == nil {
		t.Fatal("大小不符应当报错")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("大小不符时应当把半成品删掉")
	}
}

func TestSwapReplacesAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "app.exe")
	downloaded := filepath.Join(dir, "app.exe.new")
	backup := filepath.Join(dir, "app.exe.old")

	write(t, current, "old")
	write(t, downloaded, "new")

	if err := Swap(current, downloaded, backup); err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if got := read(t, current); got != "new" {
		t.Fatalf("换完之后 current 应当是 new，得到 %q", got)
	}
	if got := read(t, backup); got != "old" {
		t.Fatalf("旧程序应当被留成备份，得到 %q", got)
	}

	CleanupStale(current)
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("CleanupStale 应当清掉备份")
	}
	if got := read(t, current); got != "new" {
		t.Fatalf("清理不该动到正在用的程序，得到 %q", got)
	}
}

// 第二步失败时必须把旧程序改回来，不能把用户留在「原来的位置没有 exe」的状态。
func TestSwapRollsBackWhenDownloadMissing(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "app.exe")
	downloaded := filepath.Join(dir, "app.exe.new") // 故意不创建
	backup := filepath.Join(dir, "app.exe.old")

	write(t, current, "old")

	if err := Swap(current, downloaded, backup); err == nil {
		t.Fatal("新程序不存在时应当报错")
	}
	if got := read(t, current); got != "old" {
		t.Fatalf("失败后 current 应当被还原成 old，得到 %q", got)
	}
}

// TestLiveLatest 打真实的 GitHub API。默认跳过：它依赖网络（本机还可能被墙），
// 只在需要确认「接口形状没变」时手动跑一次：
//
//	UPDATE_LIVE_TEST=1 go test ./internal/update/ -run TestLiveLatest -v
func TestLiveLatest(t *testing.T) {
	if os.Getenv("UPDATE_LIVE_TEST") == "" {
		t.Skip("设置 UPDATE_LIVE_TEST=1 才会真的访问 GitHub")
	}
	rel, err := Latest("0.0.1")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel == nil {
		t.Fatal("公开仓库上应当至少有一个正式 release")
	}
	t.Logf("发现 v%s：%d 字节，sha256=%q\n页面 %s\n下载 %s",
		rel.Version, rel.Size, rel.SHA256, rel.PageURL, rel.URL)

	if rel.Size <= 0 {
		t.Fatal("附件大小没解析出来")
	}
	if rel.SHA256 == "" {
		t.Log("注意：这次发布的附件没有 digest 字段，下载时只能靠字节数校验")
	}
	if err := checkAssetURL(rel.URL); err != nil {
		t.Fatalf("下载地址不合规：%v", err)
	}

	// 真下一次，验证重定向、字节数与 sha256 都跑得通。
	// 只写到临时目录，绝不碰正在运行的那个 exe。
	dest := filepath.Join(t.TempDir(), "Vibe-Manager.exe")
	if err := Download(rel, dest); err != nil {
		t.Fatalf("真实下载失败：%v", err)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("真实下载并校验通过：%d 字节", fi.Size())
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
