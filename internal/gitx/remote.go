package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"vibe-manager/internal/execx"
	"vibe-manager/internal/model"
)

// remoteTimeout 是网络相关操作的兜底超时。
// 给得比较宽，因为可能弹凭据窗等用户输入；但不能没有，否则网络卡住会永远挂着。
const remoteTimeout = 5 * time.Minute

// tailChars 是回传给界面的 git 原始输出的长度上限。
const tailChars = 4000

// runRemote 跑一次可能需要联网的 git 命令：带超时，其余与 run 一致（管道收集、不弹控制台）。
//
// 刻意不做的事：
//   - 不设 GIT_TERMINAL_PROMPT=0：允许 git 自己弹凭据窗口（凭据归 git 管）
//   - 不注入任何 GIT_* / http.proxy 覆盖：让仓库自身的配置生效
//     （有的仓库把 GitHub 代理写在 http.proxy 里，覆盖了就连不上）
func runRemote(dir string, args ...string) (string, string, error) {
	cmd := exec.Command("git", args...)
	execx.Hide(cmd)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb

	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return out.String(), errb.String(), err
	case <-time.After(remoteTimeout):
		_ = cmd.Process.Kill()
		<-done // 等输出搬运 goroutine 收尾，之后读 buffer 才安全
		return out.String(), errb.String(),
			fmt.Errorf("操作超过 %s 仍未完成：可能在等待凭据输入（看看是否弹出了凭据窗口），也可能是网络或代理不通", remoteTimeout)
	}
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= tailChars {
		return s
	}
	return "…" + string(r[len(r)-tailChars:])
}

// Explain 把 git 的常见报错翻成人话；认不出来时返回空串，由调用方退回原始输出。
func Explain(stderr string) string {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "could not read username"),
		strings.Contains(s, "authentication failed"),
		strings.Contains(s, "terminal prompts disabled"),
		strings.Contains(s, "invalid username or password"),
		strings.Contains(s, "permission denied (publickey)"):
		return "认证失败：请先在命令行里成功 push/pull 一次完成登录（或 gh auth login），本工具直接复用 git 的凭据"
	case strings.Contains(s, "non-fast-forward"),
		strings.Contains(s, "fetch first"),
		strings.Contains(s, "[rejected]"),
		strings.Contains(s, "failed to push some refs"):
		return "推送被拒绝：远端有本地没有的提交，先「拉取」再推送"
	case strings.Contains(s, "no configured push destination"),
		strings.Contains(s, "has no upstream branch"),
		strings.Contains(s, "no upstream"):
		return "这个分支还没有上游：点「推送并设为上游」"
	case strings.Contains(s, "could not resolve host"),
		strings.Contains(s, "connection refused"),
		strings.Contains(s, "failed to connect"),
		strings.Contains(s, "operation timed out"),
		strings.Contains(s, "proxy"):
		return "连不上远端：检查网络与代理（本仓库 git 配置里的 http.proxy 会被沿用）"
	case strings.Contains(s, "repository not found"),
		strings.Contains(s, "does not appear to be a git repository"),
		strings.Contains(s, "not found"):
		return "远端仓库不存在或没有权限：检查 remote 地址"
	case strings.Contains(s, "would be overwritten by merge"),
		strings.Contains(s, "would be overwritten by checkout"),
		strings.Contains(s, "local changes"),
		strings.Contains(s, "please commit your changes or stash"):
		return "本地有未提交改动会被覆盖：先「打点」或「撤销改动」"
	case strings.Contains(s, "refusing to merge unrelated histories"):
		return "两个仓库历史无关：需要先在命令行用 --allow-unrelated-histories 处理"
	case strings.Contains(s, "conflict"):
		return "有冲突：需要在命令行里手工解决"
	case strings.Contains(s, "does not exist") && strings.Contains(s, "upstream"):
		return "远端还没有这个分支：先「抓取」，或直接用「推送并设为上游」"
	}
	return ""
}

// opError 依据 git 的 stderr 造一个可读的错误。
func opError(action, stderr string) error {
	if msg := Explain(stderr); msg != "" {
		return fmt.Errorf("%s失败：%s", action, msg)
	}
	body := tail(stderr)
	if body == "" {
		body = "git 没有给出更多信息"
	}
	return fmt.Errorf("%s失败：%s", action, body)
}

// conflictFiles 从 git 输出里挑出冲突文件，兼容 git 的几种写法：
//
//	CONFLICT (content): Merge conflict in a.txt
//	CONFLICT (modify/delete): b.txt deleted in HEAD and modified in other
//		both modified:   c.txt
func conflictFiles(output string) []string {
	seen := map[string]bool{}
	var files []string
	add := func(p string) {
		p = strings.TrimSpace(strings.Trim(p, `"'`))
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		files = append(files, p)
	}

	for _, line := range strings.Split(output, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "CONFLICT") {
			if i := strings.Index(l, "Merge conflict in "); i >= 0 {
				add(l[i+len("Merge conflict in "):])
				continue
			}
			// CONFLICT (modify/delete): <路径> deleted in ...
			if i := strings.Index(l, "): "); i >= 0 {
				rest := l[i+3:]
				if j := strings.IndexByte(rest, ' '); j > 0 {
					add(rest[:j])
				} else {
					add(rest)
				}
			}
			continue
		}
		for _, marker := range []string{"both modified:", "both added:", "deleted by us:", "deleted by them:", "added by us:", "added by them:"} {
			if i := strings.Index(l, marker); i >= 0 {
				add(l[i+len(marker):])
				break
			}
		}
	}
	return files
}

/* ---------------- 远程 ---------------- */

// Remotes 列出远程仓库，并带上仓库/全局配置里的 http.proxy（界面上要提示走代理）。
func Remotes(dir string) ([]model.Remote, error) {
	out, errs, err := run(dir, "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errs))
	}
	proxy := proxyOf(dir)
	seen := map[string]int{}
	var list []model.Remote
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name, url := fields[0], fields[1]
		if i, ok := seen[name]; ok {
			// 同一远程会有 fetch/push 两行，优先展示 fetch 的那条地址
			if strings.Contains(line, "(fetch)") {
				list[i].URL = url
			}
			continue
		}
		seen[name] = len(list)
		list = append(list, model.Remote{Name: name, URL: url, Proxy: proxy})
	}
	return list, nil
}

func proxyOf(dir string) string {
	out, _, err := run(dir, "config", "--get", "http.proxy")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Fetch 抓取远端更新。remote 为空表示 --all。
func Fetch(dir, remote string, prune bool) (model.OpResult, error) {
	args := []string{"fetch"}
	if strings.TrimSpace(remote) == "" {
		args = append(args, "--all")
	} else {
		args = append(args, remote)
	}
	if prune {
		args = append(args, "--prune")
	}
	out, errs, err := runRemote(dir, args...)
	res := model.OpResult{Output: tail(out + errs)}
	if err != nil {
		return res, opError("抓取", errs)
	}
	res.Message = fetchSummary(out + errs)
	return res, nil
}

func fetchSummary(output string) string {
	n := 0
	for _, line := range strings.Split(output, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "*") || strings.Contains(l, "->") {
			n++
		}
	}
	if n == 0 {
		return "已是最新，没有新的提交"
	}
	return fmt.Sprintf("抓取完成：%d 个引用有更新", n)
}

// Pull 拉取远端更新。rebase 为真时用 --rebase。
func Pull(dir, remote, branch string, rebase bool) (model.OpResult, error) {
	args := []string{"pull"}
	if rebase {
		args = append(args, "--rebase")
	}
	if strings.TrimSpace(remote) != "" {
		args = append(args, remote)
		if strings.TrimSpace(branch) != "" {
			args = append(args, branch)
		}
	}
	out, errs, err := runRemote(dir, args...)
	combined := out + errs
	res := model.OpResult{Output: tail(combined)}
	if err != nil {
		if files := conflictFiles(combined); len(files) > 0 {
			return res, fmt.Errorf("拉取产生冲突，需要在命令行里手工解决：\n%s", strings.Join(files, "\n"))
		}
		return res, opError("拉取", errs)
	}
	if files := conflictFiles(combined); len(files) > 0 {
		return res, fmt.Errorf("拉取产生冲突，需要在命令行里手工解决：\n%s", strings.Join(files, "\n"))
	}
	res.Message = "拉取完成"
	return res, nil
}

// Push 推送当前分支。setUpstream 为真时用 -u（顺带建立上游关系）。
func Push(dir, remote, branch string, setUpstream bool) (model.OpResult, error) {
	if strings.TrimSpace(remote) == "" {
		return model.OpResult{}, fmt.Errorf("这个项目还没有远程仓库，先在命令行执行 git remote add origin <地址>")
	}
	args := []string{"push"}
	if setUpstream {
		args = append(args, "--set-upstream")
	}
	args = append(args, remote)
	if strings.TrimSpace(branch) != "" {
		args = append(args, branch)
	}
	out, errs, err := runRemote(dir, args...)
	combined := out + errs
	res := model.OpResult{Output: tail(combined)}
	if err != nil {
		return res, opError("推送", errs)
	}
	switch {
	case strings.Contains(combined, "Everything up-to-date"):
		res.Message = "远端已是最新，无需推送"
	default:
		res.Message = "已推送到 " + remote
		if setUpstream {
			res.Message += "，并已建立上游关系"
		}
	}
	return res, nil
}

// SetUpstream 只把本地分支关联到已存在的远端分支（不推送）。
func SetUpstream(dir, remote, branch string) (model.OpResult, error) {
	if strings.TrimSpace(remote) == "" || strings.TrimSpace(branch) == "" {
		return model.OpResult{}, fmt.Errorf("需要先指定远程与分支")
	}
	_, errs, err := run(dir, "branch", "--set-upstream-to="+remote+"/"+branch, branch)
	if err != nil {
		return model.OpResult{Output: tail(errs)}, opError("设置上游", errs)
	}
	return model.OpResult{Message: fmt.Sprintf("已把 %s 关联到 %s/%s", branch, remote, branch)}, nil
}
