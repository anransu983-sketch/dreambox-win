package mihomo

// 内核释放：把嵌在程序里的 mihomo.exe 与 wintun.dll 解压到数据目录 bin/。
// 用 .version 标记避免每次启动重复解压。

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"dreambox-win/internal/paths"
)

// binVersion 内核版本标记，升级内核时改这里即可触发重新释放
const binVersion = "mihomo-1.19.32/wintun-0.14.1"

// EnsureBins 确保内核文件已释放
func EnsureBins() error {
	binDir := paths.BinDir()
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	mark := filepath.Join(binDir, ".version")
	if cur, _ := os.ReadFile(mark); string(cur) == binVersion {
		return nil
	}
	if err := unzipOne(mihomoZip, "mihomo-windows-amd64.exe", paths.MihomoExe()); err != nil {
		return fmt.Errorf("释放 mihomo 失败: %w", err)
	}
	if err := unzipOne(wintunZip, "wintun/bin/amd64/wintun.dll", paths.WintunDll()); err != nil {
		return fmt.Errorf("释放 wintun 失败: %w", err)
	}
	if err := os.WriteFile(mark, []byte(binVersion), 0o644); err != nil {
		return err
	}
	return nil
}

// unzipOne 从 zip 数据中取出指定成员写到目标路径
func unzipOne(zipData []byte, member, dst string) error {
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		if f.Name != member {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, rc)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	return fmt.Errorf("zip 中未找到 %s", member)
}
