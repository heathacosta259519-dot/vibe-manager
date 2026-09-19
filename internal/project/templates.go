package project

import "fmt"

func readmeTemplate(name string) string {
	return fmt.Sprintf(`# %s

（在这里写下这个项目要做什么）

## 运行

（补充运行方式）

## 目录

- `+"`src/`"+` —— 源码
`, name)
}

func gitignoreTemplate() string {
	return `# 依赖
node_modules/
.venv/
__pycache__/
target/

# 构建产物
dist/
build/
out/
*.exe
*.o
*.obj
*.class

# 编辑器
.vscode/
.idea/
*.swp

# 系统
Thumbs.db
.DS_Store
desktop.ini

# 环境与密钥
.env
.env.local
*.pem
`
}
