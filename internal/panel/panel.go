package panel

// 多面板：metacubexd / yacd / zashboard 前端 dist 打包进程序，
// 用户在设置页三选一；所选面板释放到数据目录，
// mihomo 以 external-ui 方式在 http://127.0.0.1:9090/ui/ 提供服务。

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"dreambox-win/internal/paths"
)

//go:embed assets/metacubexd.tgz
var metacubexdTgz []byte

//go:embed assets/yacd.tgz
var yacdTgz []byte

//go:embed assets/zashboard.tgz
var zashboardTgz []byte

//go:embed assets/LICENSE
var metacubexdLicense []byte

//go:embed assets/zashboard-LICENSE.txt
var zashboardLicense []byte

//go:embed assets/yacd-ATTRIBUTION.txt
var yacdAttribution []byte

// Panel 面板元信息
type Panel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Author  string `json:"author"`
	License string `json:"license"`
	Home    string `json:"home"`

	data        []byte
	licenseFile []byte // 释放到面板目录的 LICENSE 内容
	attribution string // 释放到面板目录的 ATTRIBUTION.txt 内容
}

// DefaultID 默认面板
const DefaultID = "metacubexd"

// registry 面板注册表
var registry = []Panel{
	{
		ID:      "metacubexd",
		Name:    "Meta 面板",
		Version: "1.273.1",
		Author:  "MetaCubeX",
		License: "MIT，Copyright (c) 2023 MetaCubeX",
		Home:    "https://github.com/MetaCubeX/metacubexd",
		data:    metacubexdTgz, licenseFile: metacubexdLicense,
		attribution: "专业面板基于 metacubexd v1.273.1\n上游项目：https://github.com/MetaCubeX/metacubexd\n许可证：MIT License，Copyright (c) 2023 MetaCubeX（见同目录 LICENSE 文件）\n",
	},
	{
		ID:      "yacd",
		Name:    "Yacd 经典",
		Version: "gh-pages 2026-10-05",
		Author:  "MetaCubeX（yacd 分支）/ hacdias",
		License: "上游未附 LICENSE；原 yacd 为 MIT（hacdias）",
		Home:    "https://github.com/MetaCubeX/Yacd-meta",
		data:    yacdTgz, licenseFile: nil,
		attribution: string(yacdAttribution),
	},
	{
		ID:      "zashboard",
		Name:    "Zashboard",
		Version: "3.29.1",
		Author:  "Zephyruso",
		License: "MIT，Copyright 2024 Zephyruso",
		Home:    "https://github.com/Zephyruso/zashboard",
		data:    zashboardTgz, licenseFile: zashboardLicense,
		attribution: "专业面板基于 zashboard v3.29.1\n上游项目：https://github.com/Zephyruso/zashboard\n许可证：MIT License，Copyright 2024 Zephyruso（见同目录 LICENSE 文件）\n",
	},
}

// Panels 返回全部面板元信息（不含二进制）
func Panels() []Panel {
	out := make([]Panel, len(registry))
	for i, p := range registry {
		p.data, p.licenseFile = nil, nil
		out[i] = p
	}
	return out
}

// PanelByID 按 id 查找面板，非法 id 返回错误
func PanelByID(id string) (*Panel, error) {
	for i := range registry {
		if registry[i].ID == id {
			return &registry[i], nil
		}
	}
	return nil, fmt.Errorf("未知面板: %s", id)
}

// ValidID id 是否合法
func ValidID(id string) bool {
	_, err := PanelByID(id)
	return err == nil
}

// versionMark 版本标记（面板 id+版本），变化时触发重新释放
func versionMark(p *Panel) string { return p.ID + ":" + p.Version }

// EnsurePanel 确保所选面板已释放到数据目录；非法 id 返回错误
func EnsurePanel(id string) error {
	p, err := PanelByID(id)
	if err != nil {
		return err
	}
	return ensurePanelTo(p, paths.PanelDir())
}

func ensurePanelTo(p *Panel, dir string) error {
	if len(p.data) == 0 {
		return fmt.Errorf("面板 %s 资源为空", p.ID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建面板目录失败: %w", err)
	}
	mark := filepath.Join(dir, ".version")
	if cur, _ := os.ReadFile(mark); string(cur) == versionMark(p) {
		return nil // 已是最新，无需重复解压
	}
	// 面板切换或版本变化：清空旧文件后释放
	if err := cleanDir(dir); err != nil {
		return fmt.Errorf("清理旧面板失败: %w", err)
	}
	if err := extractTgz(p.data, dir); err != nil {
		return fmt.Errorf("释放面板失败: %w", err)
	}
	// 许可证与署名随面板目录一并保留
	if len(p.licenseFile) > 0 {
		_ = os.WriteFile(filepath.Join(dir, "LICENSE"), p.licenseFile, 0o644)
	}
	_ = os.WriteFile(filepath.Join(dir, "ATTRIBUTION.txt"), []byte(p.attribution), 0o644)
	if err := os.WriteFile(mark, []byte(versionMark(p)), 0o644); err != nil {
		return err
	}
	return nil
}

// cleanDir 清空目录内所有文件（保留目录本身）
func cleanDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// extractTgz 解压 tgz 到目标目录（防 tar-slip）
func extractTgz(data []byte, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		name = filepath.Clean(name)
		if name == "." || name == "" || strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(dst, name)
		// 确保目标仍在 dst 内
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) && target != filepath.Clean(dst) {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			closeErr := out.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}
