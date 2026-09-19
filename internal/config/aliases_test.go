package config

import (
	"strings"
	"testing"
)

func TestAliasesRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	if got := Aliases(); len(got) != 0 {
		t.Fatalf("初始应为空：%v", got)
	}
	if err := SetAlias(`X:\a\proj`, "我的项目"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got := Aliases()
	if got[NormalizePath(`X:\a\proj`)] != "我的项目" {
		t.Fatalf("别名没读到：%v", got)
	}
	// 大小写不同也应当命中同一个键（Windows 路径不区分大小写）
	if got[NormalizePath(`X:\A\PROJ`)] != "我的项目" {
		t.Fatalf("路径键没做大小写归一：%v", got)
	}

	if err := SetAlias(`X:\a\proj`, "  "); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(Aliases()) != 0 {
		t.Fatalf("清除后应为空：%v", Aliases())
	}
}

func TestProjectStoreMigrationFromLegacyFormat(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	// 旧格式：整份文件就是一个「路径 → 备注名」的扁平映射
	if err := writeRaw(AliasesPath(), `{"X:\\a\\proj":"老备注"}`); err != nil {
		t.Fatal(err)
	}
	if got := Aliases()[NormalizePath(`X:\a\proj`)]; got != "老备注" {
		t.Fatalf("旧格式没迁移过来：%q", got)
	}
	// 迁移后再写入，应当变成新格式且不丢数据
	if err := SetProjectMeta(`X:\a\proj`, "新备注", `X:\a\proj\sub`); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := Aliases()[NormalizePath(`X:\a\proj`)]; got != "新备注" {
		t.Fatalf("备注名不对：%q", got)
	}
	if got := Roots()[NormalizePath(`X:\a\proj`)]; got != NormalizePath(`X:\a\proj\sub`) {
		t.Fatalf("实际根不对：%q", got)
	}
}

func TestRootsAndImports(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	// 实际根与条目相同时不记录（等于没设）
	if err := SetProjectMeta(`X:\a\proj`, "", `X:\a\proj`); err != nil {
		t.Fatal(err)
	}
	if len(Roots()) != 0 {
		t.Fatalf("相同的根不该被记录：%v", Roots())
	}

	if err := SetProjectMeta(`X:\a\proj`, "", `X:\a\proj\WinGlass`); err != nil {
		t.Fatal(err)
	}
	if got := Roots()[NormalizePath(`X:\a\proj`)]; got != NormalizePath(`X:\a\proj\WinGlass`) {
		t.Fatalf("实际根没记上：%v", Roots())
	}
	// 只改备注名时，实际根要保住
	if err := SetAlias(`X:\a\proj`, "备注"); err != nil {
		t.Fatal(err)
	}
	if len(Roots()) != 1 {
		t.Fatalf("改备注名不该丢掉实际根：%v", Roots())
	}
	// 清空实际根
	if err := SetProjectMeta(`X:\a\proj`, "备注", ""); err != nil {
		t.Fatal(err)
	}
	if len(Roots()) != 0 {
		t.Fatalf("实际根应被清除：%v", Roots())
	}

	// 导入 / 移出
	if err := AddImport(`X:\outside\repo`); err != nil {
		t.Fatal(err)
	}
	if err := AddImport(`X:\outside\repo`); err != nil {
		t.Fatalf("重复导入不应报错：%v", err)
	}
	if n := len(LoadProjectStore().Imports); n != 1 {
		t.Fatalf("重复导入应去重，得到 %d 条", n)
	}
	if !IsRegisteredPath(`X:\outside\repo`) {
		t.Fatal("导入的路径应当算已注册")
	}
	if err := RemoveImport(`X:\OUTSIDE\REPO`); err != nil {
		t.Fatalf("移出: %v", err)
	}
	if IsRegisteredPath(`X:\outside\repo`) {
		t.Fatal("移出后不应再算已注册")
	}
}

func TestIsRegisteredPath(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	if IsRegisteredPath("") || IsRegisteredPath(`X:\nope`) {
		t.Fatal("未注册的路径不该通过")
	}
	if err := SetProjectMeta(`X:\a\proj`, "备注", `X:\a\proj\sub`); err != nil {
		t.Fatal(err)
	}
	// 条目位置与实际根都算已注册
	if !IsRegisteredPath(`X:\a\proj`) {
		t.Fatal("条目位置应当算已注册")
	}
	if !IsRegisteredPath(`X:\A\PROJ\SUB`) {
		t.Fatal("实际根应当算已注册（且大小写不敏感）")
	}
	if IsRegisteredPath(`X:\a\proj\sub\deeper`) {
		t.Fatal("实际根的下级不该算已注册")
	}
}

func TestStoreToleratesBrokenFile(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if err := writeRaw(AliasesPath(), "{不是合法 json"); err != nil {
		t.Fatal(err)
	}
	if got := LoadProjectStore(); len(got.Aliases) != 0 || len(got.Roots) != 0 || len(got.Imports) != 0 {
		t.Fatalf("坏文件应退化为空：%+v", got)
	}
	if err := SetAlias(`X:\a\proj`, "备注"); err != nil {
		t.Fatalf("坏文件后仍应能写入：%v", err)
	}
}

func TestSanitizeFont(t *testing.T) {
	if got := SanitizeFont("Consolas"); got != "Consolas" {
		t.Fatalf("普通字体名不该被改：%q", got)
	}
	if got := SanitizeFont("Consolas, 微软雅黑"); got != "Consolas, 微软雅黑" {
		t.Fatalf("逗号与中文应保留：%q", got)
	}
	bad := SanitizeFont(`X; } body { color: red } /*`)
	if strings.ContainsAny(bad, `;{}()<>"'\`) {
		t.Fatalf("危险字符没清干净：%q", bad)
	}
	if got := SanitizeFont(strings.Repeat("a", 500)); len(got) > 120 {
		t.Fatalf("超长字体名应被截断：%d", len(got))
	}
}

func TestClampEditorWidth(t *testing.T) {
	cases := map[int]int{0: DefaultEditorWidth, -5: DefaultEditorWidth, 10: MinEditorWidth, 5000: MaxEditorWidth, 600: 600}
	for in, want := range cases {
		if got := ClampEditorWidth(in); got != want {
			t.Fatalf("ClampEditorWidth(%d) = %d，期望 %d", in, got, want)
		}
	}
}
