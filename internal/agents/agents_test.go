package agents

import (
	"strings"
	"testing"
)

func TestBuiltinsRenderWithName(t *testing.T) {
	list := List()
	if len(list) < 4 {
		t.Fatalf("内置模板应有多份，得到 %d", len(list))
	}
	for _, tpl := range list {
		if tpl.ID == "" || tpl.Name == "" || tpl.Body == "" {
			t.Fatalf("模板字段不完整：%+v", tpl)
		}
		if !tpl.Builtin {
			t.Fatalf("内置列表里出现了非内置模板：%+v", tpl)
		}
		body := Render(tpl.ID, "my-demo")
		if !strings.Contains(body, "my-demo") {
			t.Fatalf("%s 没替换项目名：%s", tpl.ID, body[:60])
		}
		if strings.Contains(body, "{{NAME}}") {
			t.Fatalf("%s 还有未替换的占位符", tpl.ID)
		}
		if !strings.HasSuffix(body, "\n") {
			t.Fatalf("%s 应以换行结尾", tpl.ID)
		}
	}
}

func TestRenderFallbackAndUnknown(t *testing.T) {
	// 空 id 用默认模板
	if got := Render("", "x"); !strings.Contains(got, "x") {
		t.Fatalf("空 id 应回退到默认模板：%q", got)
	}
	// 未知 id 返回空串，由调用方决定回退
	if got := Render("builtin-does-not-exist", "x"); got != "" {
		t.Fatalf("未知 id 应返回空串：%q", got)
	}
}

func TestUserTemplateCRUD(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	tpl, err := SaveUser("", "  我的模板  ", "内容 A\n")
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if tpl.ID == "" || tpl.Builtin {
		t.Fatalf("新建的用户模板不对：%+v", tpl)
	}
	if !strings.Contains(Render(tpl.ID, "p"), "内容 A") {
		t.Fatal("用户模板没被 Render 认到")
	}

	// 覆盖
	if _, err := SaveUser(tpl.ID, "我的模板2", "内容 B\n"); err != nil {
		t.Fatalf("覆盖: %v", err)
	}
	got := Render(tpl.ID, "p")
	if !strings.Contains(got, "内容 B") || strings.Contains(got, "内容 A") {
		t.Fatalf("覆盖没生效：%q", got)
	}
	if n := countUser(); n != 1 {
		t.Fatalf("覆盖不应新增，得到 %d 份", n)
	}

	// 删除
	if err := DeleteUser(tpl.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n := countUser(); n != 0 {
		t.Fatalf("删除后应为 0 份，得到 %d", n)
	}
	if err := DeleteUser(tpl.ID); err == nil {
		t.Fatal("重复删除应当报错")
	}
}

func TestUserTemplateGuards(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	if _, err := SaveUser("", "   ", "内容"); err == nil {
		t.Fatal("空模板名应被拒绝")
	}
	if _, err := SaveUser("", "名字", "   \n"); err == nil {
		t.Fatal("空内容应被拒绝")
	}
	if _, err := SaveUser("builtin-general", "伪装", "内容"); err == nil {
		t.Fatal("不许覆盖内置模板")
	}
	if err := DeleteUser("builtin-general"); err == nil {
		t.Fatal("不许删除内置模板")
	}
}

func countUser() int {
	n := 0
	for _, tpl := range List() {
		if !tpl.Builtin {
			n++
		}
	}
	return n
}
