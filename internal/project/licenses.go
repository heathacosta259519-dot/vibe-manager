package project

import (
	"embed"
	"fmt"
	"strings"
	"time"

	"vibe-manager/internal/model"
)

//go:embed licenses/*.txt
var licenseFS embed.FS

var licenseFiles = map[string]string{
	"MIT":          "licenses/mit.txt",
	"Apache-2.0":   "licenses/apache-2.0.txt",
	"BSD-3-Clause": "licenses/bsd-3-clause.txt",
	"ISC":          "licenses/isc.txt",
	"MPL-2.0":      "licenses/mpl-2.0.txt",
	"GPL-3.0":      "licenses/gpl-3.0.txt",
}

// LicenseOptions 是新建项目时可选的协议（第一项为空，表示不加协议文件）。
// 只有 MIT / ISC / BSD-3-Clause 三份是手写的短文本；
// Apache-2.0 / MPL-2.0 / GPL-3.0 三份是从本机权威副本原样拷入的完整正文。
var LicenseOptions = []model.LicenseOption{
	{ID: "", Name: "不加协议文件"},
	{ID: "MIT", Name: "MIT"},
	{ID: "Apache-2.0", Name: "Apache License 2.0"},
	{ID: "BSD-3-Clause", Name: "BSD 3-Clause"},
	{ID: "ISC", Name: "ISC"},
	{ID: "MPL-2.0", Name: "Mozilla Public License 2.0"},
	{ID: "GPL-3.0", Name: "GNU GPL v3.0"},
}

// LicenseText 返回填好年份与署名的协议正文；id 为空或未知时返回空串。
func LicenseText(id, holder string) (string, error) {
	file, ok := licenseFiles[id]
	if !ok {
		return "", nil
	}
	data, err := licenseFS.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("读取协议文本失败：%v", err)
	}
	year := fmt.Sprintf("%d", time.Now().Year())
	text := string(data)
	text = strings.ReplaceAll(text, "{{YEAR}}", year)
	text = strings.ReplaceAll(text, "{{HOLDER}}", holder)
	// Apache-2.0 末尾附录里的占位符
	text = strings.ReplaceAll(text, "[yyyy] [name of copyright owner]", year+" "+holder)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text, nil
}

// LicenseName 把 id 翻成展示名。
func LicenseName(id string) string {
	for _, o := range LicenseOptions {
		if o.ID == id {
			return o.Name
		}
	}
	return id
}
