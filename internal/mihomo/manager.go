package mihomo

// 内核进程管理：启动 / 停止 / 重启。仅 Windows 能真正运行 mihomo.exe，
// Linux 下 Start 返回错误（用于开发机调试 Web UI 时优雅降级）。
//
// 内核的 stdout/stderr 重定向到 %APPDATA%/DreamBox/mihomo.log（每次启动截断），
// 供 Web 控制台「查看内核日志」远程排障。

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"dreambox-win/internal/paths"
)

var (
	mu       sync.Mutex
	cmd      *exec.Cmd
	lastExit time.Time
	lastErr  error
)

// LastExit 返回内核最后一次退出的时间与错误；
// lastExit 为零值表示本次运行中内核尚未退出过。
func LastExit() (time.Time, error) {
	mu.Lock()
	defer mu.Unlock()
	return lastExit, lastErr
}

// Start 启动内核（先确保已生成配置）
func Start() error {
	mu.Lock()
	defer mu.Unlock()
	if runtime.GOOS != "windows" {
		return fmt.Errorf("内核仅支持在 Windows 上运行")
	}
	if cmd != nil && cmd.Process != nil {
		return nil // 已在运行
	}
	logFile, err := os.OpenFile(paths.MihomoLog(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("创建内核日志失败: %w", err)
	}
	c := exec.Command(paths.MihomoExe(), "-d", paths.DataDir())
	c.Stdout = logFile
	c.Stderr = logFile
	hideChildConsole(c) // Windows 下隐藏内核的控制台黑窗口
	if err := c.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("启动内核失败: %w", err)
	}
	cmd = c
	go func() {
		waitErr := c.Wait()
		_ = logFile.Close()
		mu.Lock()
		if cmd == c {
			cmd = nil
			lastExit = time.Now()
			lastErr = waitErr
		}
		mu.Unlock()
	}()
	return nil
}

// Stop 停止内核
func Stop() error {
	mu.Lock()
	defer mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	cmd = nil
	return nil
}

// Restart 重启内核（配置变更后调用）
func Restart() error {
	_ = Stop()
	return Start()
}

// Running 内核进程是否存活
func Running() bool {
	mu.Lock()
	defer mu.Unlock()
	return cmd != nil && cmd.Process != nil
}
