// Package agents 管理新建项目时用到的 AGENTS.md 模板。
//
// 内置若干份通用模板；用户也可以自建模板并保存，存于
// %APPDATA%\vibe-pm\agents-templates.json（不放进项目目录，避免污染仓库）。
package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"vibe-manager/internal/config"
)

// Template 是一份 AGENTS.md 模板。Body 里的 {{NAME}} 会被替换成项目名。
type Template struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
	Body    string `json:"body"`
}

type store struct {
	Templates []Template `json:"templates"`
}

const builtinPrefix = "builtin-"

// List 返回内置模板 + 用户模板（用户模板按名称排序）。
func List() []Template {
	out := make([]Template, 0, len(builtins))
	out = append(out, builtins...)
	user := loadUser()
	sort.Slice(user, func(i, j int) bool { return user[i].Name < user[j].Name })
	out = append(out, user...)
	return out
}

// Render 按模板 id 渲染出 AGENTS.md 正文；找不到时返回空串。
func Render(id, projectName string) string {
	if id == "" {
		id = DefaultID
	}
	for _, t := range List() {
		if t.ID == id {
			body := strings.ReplaceAll(t.Body, "{{NAME}}", projectName)
			if !strings.HasSuffix(body, "\n") {
				body += "\n"
			}
			return body
		}
	}
	return ""
}

// DefaultID 是默认选中的模板。
const DefaultID = "builtin-general"

// SaveUser 新建或覆盖一份用户模板；id 为空表示新建。
func SaveUser(id, name, body string) (Template, error) {
	name = strings.TrimSpace(name)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if name == "" {
		return Template{}, fmt.Errorf("模板名不能为空")
	}
	if strings.TrimSpace(body) == "" {
		return Template{}, fmt.Errorf("模板内容不能为空")
	}
	if strings.HasPrefix(id, builtinPrefix) {
		return Template{}, fmt.Errorf("内置模板不能覆盖，请用「另存为」")
	}

	user := loadUser()
	if id == "" {
		id = fmt.Sprintf("u-%d", time.Now().UnixNano())
	}
	found := false
	for i := range user {
		if user[i].ID == id {
			user[i].Name = name
			user[i].Body = body
			found = true
			break
		}
	}
	if !found {
		user = append(user, Template{ID: id, Name: name, Body: body})
	}
	if err := persist(user); err != nil {
		return Template{}, err
	}
	return Template{ID: id, Name: name, Body: body}, nil
}

// DeleteUser 删除一份用户模板。
func DeleteUser(id string) error {
	if strings.HasPrefix(id, builtinPrefix) {
		return fmt.Errorf("内置模板不能删除")
	}
	user := loadUser()
	kept := make([]Template, 0, len(user))
	hit := false
	for _, t := range user {
		if t.ID == id {
			hit = true
			continue
		}
		kept = append(kept, t)
	}
	if !hit {
		return fmt.Errorf("找不到该模板")
	}
	return persist(kept)
}

func loadUser() []Template {
	data, err := os.ReadFile(config.TemplatesPath())
	if err != nil {
		return nil
	}
	var s store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	out := make([]Template, 0, len(s.Templates))
	for _, t := range s.Templates {
		if strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Body) == "" {
			continue
		}
		if strings.HasPrefix(t.ID, builtinPrefix) {
			continue // 防止用户文件里塞内置 id 覆盖内置模板
		}
		t.Builtin = false
		out = append(out, t)
	}
	return out
}

func persist(list []Template) error {
	p := config.TemplatesPath()
	if err := os.MkdirAll(dirOf(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store{Templates: list}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func dirOf(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[:i]
	}
	return "."
}
