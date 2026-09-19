package gitx

import (
	"fmt"
	"strconv"
	"strings"

	"vibe-manager/internal/model"
)

// Branches 列出本地分支。
//
// 说明：领先/落后用 `%(upstream:trackshort)` 判断（一次进程搞定所有分支，快）；
// 它只给方向不给具体数字，所以非当前分支的 Ahead/Behind 是 0/1 标记，
// 当前分支的精确数字由 Status 提供（界面上会覆盖显示）。
func Branches(dir string) ([]model.Branch, error) {
	format := strings.Join([]string{
		"%(refname:short)",
		"%(upstream:short)",
		"%(HEAD)",
		"%(committerdate:unix)",
		"%(upstream:trackshort)",
		"%(contents:subject)",
	}, unitSep)

	out, errs, err := run(dir, "for-each-ref", "refs/heads", "--format="+format,
		"--sort=-committerdate")
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errs))
	}

	var list []model.Branch
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.SplitN(line, unitSep, 6)
		if len(f) < 6 {
			continue
		}
		when, _ := strconv.ParseInt(strings.TrimSpace(f[3]), 10, 64)
		b := model.Branch{
			Name:     f[0],
			Upstream: f[1],
			Current:  strings.TrimSpace(f[2]) == "*",
			When:     when,
			Subject:  f[5],
		}
		switch strings.TrimSpace(f[4]) {
		case ">":
			b.Ahead = 1
		case "<":
			b.Behind = 1
		case "<>":
			b.Ahead, b.Behind = 1, 1
		}
		list = append(list, b)
	}
	return list, nil
}

// Checkout 切换分支。
func Checkout(dir, name string) (model.OpResult, error) {
	if strings.TrimSpace(name) == "" {
		return model.OpResult{}, fmt.Errorf("请指定分支名")
	}
	out, errs, err := run(dir, "checkout", name)
	combined := out + errs
	res := model.OpResult{Output: tail(combined)}
	if err != nil {
		return res, opError("切换分支", errs)
	}
	res.Message = "已切换到 " + name
	return res, nil
}

// CreateBranch 新建分支，可选立即切换过去。
func CreateBranch(dir, name string, checkout bool) (model.OpResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.OpResult{}, fmt.Errorf("请指定分支名")
	}
	if strings.ContainsAny(name, " \t~^:?*[\\") {
		return model.OpResult{}, fmt.Errorf("分支名不能包含空格或 ~^:?*[\\ 这些字符")
	}
	args := []string{"branch", name}
	if checkout {
		args = []string{"checkout", "-b", name}
	}
	out, errs, err := run(dir, args...)
	combined := out + errs
	res := model.OpResult{Output: tail(combined)}
	if err != nil {
		return res, opError("新建分支", errs)
	}
	if checkout {
		res.Message = "已新建并切换到 " + name
	} else {
		res.Message = "已新建分支 " + name
	}
	return res, nil
}

// DeleteBranch 删除本地分支。force 对应 -D（丢弃未合并内容）。
func DeleteBranch(dir, name string, force bool) (model.OpResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.OpResult{}, fmt.Errorf("请指定分支名")
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	out, errs, err := run(dir, "branch", flag, name)
	combined := out + errs
	res := model.OpResult{Output: tail(combined)}
	if err != nil {
		if !force && strings.Contains(combined, "not fully merged") {
			return res, fmt.Errorf("分支 %s 还有未合并的提交，删掉会丢内容；确认不要了就再点一次「强制删除」", name)
		}
		return res, opError("删除分支", errs)
	}
	res.Message = "已删除分支 " + name
	return res, nil
}

// Merge 把 name 合并进当前分支。冲突时返回冲突文件清单。
func Merge(dir, name string) (model.OpResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.OpResult{}, fmt.Errorf("请指定要合并的分支")
	}
	out, errs, err := run(dir, "merge", "--no-edit", name)
	combined := out + errs
	res := model.OpResult{Output: tail(combined)}
	if files := conflictFiles(combined); len(files) > 0 {
		return res, fmt.Errorf("合并产生冲突，需要在命令行里手工解决：\n%s", strings.Join(files, "\n"))
	}
	if err != nil {
		return res, opError("合并", errs)
	}
	if strings.Contains(combined, "Already up to date") {
		res.Message = "已经是最新的，无需合并"
		return res, nil
	}
	res.Message = "已合并 " + name
	return res, nil
}

// Tags 列出标签。
func Tags(dir string) ([]model.Tag, error) {
	format := strings.Join([]string{
		"%(refname:short)",
		"%(objecttype)",
		"%(creatordate:unix)",
		"%(contents:subject)",
	}, unitSep)

	out, errs, err := run(dir, "for-each-ref", "refs/tags", "--format="+format, "--sort=-creatordate")
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(errs))
	}

	var list []model.Tag
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.SplitN(line, unitSep, 4)
		if len(f) < 4 {
			continue
		}
		when, _ := strconv.ParseInt(strings.TrimSpace(f[2]), 10, 64)
		list = append(list, model.Tag{
			Name:      f[0],
			Annotated: strings.TrimSpace(f[1]) == "tag",
			When:      when,
			Subject:   f[3],
		})
	}
	return list, nil
}

// CreateTag 新建标签；message 非空时为附注标签。
func CreateTag(dir, name, message string) (model.OpResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.OpResult{}, fmt.Errorf("请指定标签名")
	}
	if strings.ContainsAny(name, " \t~^:?*[\\") {
		return model.OpResult{}, fmt.Errorf("标签名不能包含空格或 ~^:?*[\\ 这些字符")
	}
	args := []string{"tag", name}
	if msg := strings.TrimSpace(message); msg != "" {
		args = []string{"tag", "-a", name, "-m", msg}
	}
	out, errs, err := run(dir, args...)
	combined := out + errs
	res := model.OpResult{Output: tail(combined)}
	if err != nil {
		return res, opError("新建标签", errs)
	}
	res.Message = "已新建标签 " + name
	return res, nil
}

// Revert 生成一个反向提交来撤销指定提交。
//
// 刻意不自动 stash：revert 只新增提交、本身就能再 revert 回去，
// 而静默 stash 会动用户的工作区，反而更让人意外。
func Revert(dir, sha string) (string, error) {
	if !IsRepo(dir) {
		return "", fmt.Errorf("这不是一个 git 仓库")
	}
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return "", fmt.Errorf("请指定要撤销的提交")
	}
	out, errs, err := run(dir, "revert", "--no-edit", sha)
	combined := out + errs
	if files := conflictFiles(combined); len(files) > 0 {
		return "", fmt.Errorf("撤销产生冲突，需要在命令行里手工解决（或 git revert --abort 放弃）：\n%s",
			strings.Join(files, "\n"))
	}
	if err != nil {
		return "", opError("撤销提交", errs)
	}
	if len(sha) > 7 {
		sha = sha[:7]
	}
	return "已生成反向提交，撤销了 " + sha, nil
}

// PushTag 把标签推到远程。
func PushTag(dir, name, remote string) (model.OpResult, error) {
	if strings.TrimSpace(remote) == "" {
		return model.OpResult{}, fmt.Errorf("这个项目还没有远程仓库")
	}
	out, errs, err := runRemote(dir, "push", remote, name)
	combined := out + errs
	res := model.OpResult{Output: tail(combined)}
	if err != nil {
		return res, opError("推送标签", errs)
	}
	res.Message = fmt.Sprintf("已把标签 %s 推到 %s", name, remote)
	return res, nil
}
