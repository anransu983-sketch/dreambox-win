package theme

// 主题系统：主题目录 %APPDATA%\DreamBox\themes\<id>\，每个主题含 theme.json + theme.css，
// CSS 里可用相对路径引用包内背景图。内置主题 go:embed，首次运行释放，上传/删除不可动内置。

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"dreambox-win/internal/paths"
)

//go:embed themes/default themes/bocchi-starlight
var builtinFS embed.FS

// builtinIDs 内置主题 id（上传/删除时拒绝占用）
var builtinIDs = []string{"default", "bocchi-starlight"}

// Meta theme.json 内容
type Meta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Description string `json:"description"`
}

// Theme 主题信息
type Theme struct {
	Meta
	Builtin bool `json:"builtin"`
	Active  bool `json:"active"`
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ValidID 校验主题 id（防目录穿越）
func ValidID(id string) bool { return idRe.MatchString(id) }

func isBuiltin(id string) bool {
	for _, b := range builtinIDs {
		if b == id {
			return true
		}
	}
	return false
}

// EnsureBuiltin 释放内置主题：目录缺失或内置版本更新时覆盖写入
func EnsureBuiltin() error {
	if err := os.MkdirAll(paths.ThemesDir(), 0o755); err != nil {
		return err
	}
	for _, id := range builtinIDs {
		cur, _ := readMeta(paths.ThemeDir(id))
		emb, err := readBuiltinMeta(id)
		if err != nil {
			return err
		}
		if cur == nil || cur.Version != emb.Version {
			if err := copyBuiltin(id); err != nil {
				return fmt.Errorf("释放内置主题 %s 失败: %w", id, err)
			}
		}
	}
	return nil
}

func readBuiltinMeta(id string) (*Meta, error) {
	data, err := builtinFS.ReadFile("themes/" + id + "/theme.json")
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func copyBuiltin(id string) error {
	dir := paths.ThemeDir(id)
	_ = os.RemoveAll(dir)
	return fs.WalkDir(builtinFS, "themes/"+id, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("themes/"+id, p)
		target := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := builtinFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func readMeta(dir string) (*Meta, error) {
	data, err := os.ReadFile(filepath.Join(dir, "theme.json"))
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.ID == "" || m.Name == "" {
		return nil, fmt.Errorf("theme.json 缺少 id/name")
	}
	return &m, nil
}

// Exists 主题是否存在
func Exists(id string) bool {
	if !ValidID(id) {
		return false
	}
	_, err := readMeta(paths.ThemeDir(id))
	return err == nil
}

// Get 读取单个主题
func Get(id string, active bool) (*Theme, error) {
	if !ValidID(id) {
		return nil, fmt.Errorf("非法主题 id")
	}
	m, err := readMeta(paths.ThemeDir(id))
	if err != nil {
		return nil, fmt.Errorf("主题不存在: %s", id)
	}
	return &Theme{Meta: *m, Builtin: isBuiltin(id), Active: active}, nil
}

// List 列出全部主题（内置优先，其次按名称）
func List(activeID string) []Theme {
	out := []Theme{}
	entries, err := os.ReadDir(paths.ThemesDir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		m, err := readMeta(paths.ThemeDir(e.Name()))
		if err != nil {
			continue
		}
		out = append(out, Theme{Meta: *m, Builtin: isBuiltin(e.Name()), Active: e.Name() == activeID})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Builtin != out[j].Builtin {
			return out[i].Builtin
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Upload 校验并安装主题 zip（包内根目录须含 theme.json + theme.css）
func Upload(data []byte) (*Theme, error) {
	if len(data) == 0 || len(data) > 20<<20 {
		return nil, fmt.Errorf("zip 包大小非法（上限 20MB）")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("不是有效的 zip 包")
	}
	var metaFile, cssFile *zip.File
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		// 只认根目录文件，拒绝嵌套与目录穿越
		if strings.Contains(name, "/") || name != filepath.Base(name) {
			continue
		}
		switch name {
		case "theme.json":
			metaFile = f
		case "theme.css":
			cssFile = f
		}
	}
	if metaFile == nil {
		return nil, fmt.Errorf("zip 包内缺少 theme.json（须放在根目录）")
	}
	if cssFile == nil {
		return nil, fmt.Errorf("zip 包内缺少 theme.css（须放在根目录）")
	}
	rc, err := metaFile.Open()
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(rc, 64<<10))
	_ = rc.Close()
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("theme.json 解析失败")
	}
	if !ValidID(m.ID) {
		return nil, fmt.Errorf("theme.json 的 id 非法（仅允许小写字母/数字/连字符）")
	}
	if m.Name == "" {
		return nil, fmt.Errorf("theme.json 缺少 name")
	}
	if isBuiltin(m.ID) {
		return nil, fmt.Errorf("不可覆盖内置主题")
	}
	dir := paths.ThemeDir(m.ID)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("主题 id 已存在：%s", m.ID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// 解压全部根目录文件（zip slip 防护 + 单文件 5MB 上限）
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if strings.Contains(name, "/") || name != filepath.Base(name) || name == "" {
			continue
		}
		if f.UncompressedSize64 > 5<<20 {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("文件过大：%s", name)
		}
		if err := extractFile(f, filepath.Join(dir, name)); err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
	}
	return &Theme{Meta: m, Builtin: false, Active: false}, nil
}

func extractFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(rc, 5<<20+1))
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

// Delete 删除主题（内置不可删）
func Delete(id string) error {
	if !ValidID(id) {
		return fmt.Errorf("非法主题 id")
	}
	if isBuiltin(id) {
		return fmt.Errorf("内置主题不可删除")
	}
	dir := paths.ThemeDir(id)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("主题不存在: %s", id)
	}
	return os.RemoveAll(dir)
}
