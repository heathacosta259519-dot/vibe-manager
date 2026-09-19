// Package update 检查公开仓库有没有新版本，并把新版本的可执行文件换上去。
//
// 更新源是写死的固定仓库，不从配置读——否则「谁能让程序下载并运行一个 exe」
// 就变成了一个可被配置改写的攻击面。
package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Repo 是公开仓库的 owner/name。
const Repo = "heathacosta259519-dot/vibe-manager"

// AssetName 是发布附件里要下载的那个文件。
const AssetName = "Vibe-Manager.exe"

// 抽成变量是为了让测试能把它们指向本地 httptest 服务器。
var (
	apiBase = "https://api.github.com"
	// 只允许从 GitHub 自己的域名取文件；API 返回别处的地址一律拒绝。
	allowedHosts = map[string]bool{
		"github.com":                     true,
		"api.github.com":                 true,
		"objects.githubusercontent.com":  true,
		"release-assets.githubusercontent.com": true,
	}

	checkClient    = &http.Client{Timeout: 12 * time.Second}
	downloadClient = &http.Client{Timeout: 10 * time.Minute}
)

// Release 是我们从 release 里关心的那几项。
type Release struct {
	Version string // 去掉 v 前缀，如 "0.6.0"
	Tag     string // 原始 tag，如 "v0.6.0"
	Notes   string // release 正文
	PageURL string
	URL     string // exe 下载地址
	Size    int64
	SHA256  string // 附件摘要（十六进制小写），空表示这次发布没给
}

type apiAsset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}

type apiRelease struct {
	Tag     string     `json:"tag_name"`
	Body    string     `json:"body"`
	HTMLURL string     `json:"html_url"`
	Assets  []apiAsset `json:"assets"`
}

// Latest 查询最新正式版。没有比 current 更高的版本时返回 nil, nil。
func Latest(current string) (*Release, error) {
	req, err := http.NewRequest(http.MethodGet, apiBase+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Vibe-Manager")

	resp, err := checkClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上更新服务器：%v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// 仓库还没有任何 release（或全被删了），当作「没有更新」。
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("更新服务器返回 %d", resp.StatusCode)
	}

	var rel apiRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("看不懂更新信息：%v", err)
	}

	version := strings.TrimPrefix(strings.TrimSpace(rel.Tag), "v")
	if version == "" || Compare(version, current) <= 0 {
		return nil, nil
	}

	var asset *apiAsset
	for i := range rel.Assets {
		if strings.EqualFold(rel.Assets[i].Name, AssetName) {
			asset = &rel.Assets[i]
			break
		}
	}
	if asset == nil {
		return nil, fmt.Errorf("新版本 %s 没有附带 %s", version, AssetName)
	}
	if err := checkAssetURL(asset.URL); err != nil {
		return nil, err
	}

	return &Release{
		Version: version,
		Tag:     rel.Tag,
		Notes:   rel.Body,
		PageURL: rel.HTMLURL,
		URL:     asset.URL,
		Size:    asset.Size,
		SHA256:  strings.TrimPrefix(strings.TrimSpace(asset.Digest), "sha256:"),
	}, nil
}

// checkAssetURL 确保下载地址确实是 GitHub 自己的地址（https）。
func checkAssetURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("下载地址无法解析：%v", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("下载地址不是 https：%s", raw)
	}
	if !allowedHosts[strings.ToLower(u.Hostname())] {
		return fmt.Errorf("下载地址不在 GitHub 域名下：%s", u.Hostname())
	}
	return nil
}

// Compare 比较两个版本号，a>b 返回 1，相等返回 0，a<b 返回 -1。
// 只比较点分段里的数字（"1.2.3-rc1" 取 1.2.3），缺的段按 0 算。
func Compare(a, b string) int {
	as := versionParts(a)
	bs := versionParts(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	out := []int{}
	for _, seg := range strings.Split(v, ".") {
		digits := seg
		for i, r := range seg {
			if r < '0' || r > '9' {
				digits = seg[:i]
				break
			}
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

// Download 把 rel 的附件下到 dest，并核对字节数与 sha256。
// 任何校验不过都会把 dest 删掉，绝不留下半个文件。
func Download(rel *Release, dest string) error {
	req, err := http.NewRequest(http.MethodGet, rel.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Vibe-Manager")

	resp, err := downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：服务器返回 %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("无法写入 %s：%v", dest, err)
	}

	sum := sha256.New()
	written, err := io.Copy(io.MultiWriter(f, sum), resp.Body)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("下载中断：%v", err)
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("写入失败：%v", closeErr)
	}

	fail := func(format string, args ...interface{}) error {
		_ = os.Remove(dest)
		return fmt.Errorf(format, args...)
	}
	if rel.Size > 0 && written != rel.Size {
		return fail("下载不完整：得到 %d 字节，期望 %d", written, rel.Size)
	}
	if got := hex.EncodeToString(sum.Sum(nil)); rel.SHA256 != "" && !strings.EqualFold(got, rel.SHA256) {
		return fail("校验不通过：下载内容与发布摘要不一致")
	}
	return nil
}

// Swap 把下载好的新程序换到 current 的位置。
// 先给旧程序改名（Windows 允许给正在运行的程序改名），再让新程序顶上；
// 第二步失败就把旧程序改回来，避免把用户留在「原来的位置没有 exe」的状态。
func Swap(current, downloaded, backup string) error {
	_ = os.Remove(backup)
	if err := os.Rename(current, backup); err != nil {
		return fmt.Errorf("无法让出程序位置（可能被占用或没有权限）：%v", err)
	}
	if err := os.Rename(downloaded, current); err != nil {
		_ = os.Rename(backup, current)
		return fmt.Errorf("新程序就位失败，已还原旧程序：%v", err)
	}
	return nil
}

// CleanupStale 删掉上次更新留下的备份与残留下载。删不掉就算了，不值得打扰用户。
func CleanupStale(exePath string) {
	for _, p := range []string{exePath + ".old", exePath + ".new"} {
		if _, err := os.Stat(p); err == nil {
			_ = os.Remove(p)
		}
	}
}
