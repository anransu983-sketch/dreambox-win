package mihomo

// 内核文件以 zip 形式编译进主程序，首次运行时释放到数据目录。
// mihomo: 官方 Windows amd64 构建 v1.19.32
// wintun: wintun-0.14.1 amd64（TUN system 栈必需）

import _ "embed"

//go:embed assets/mihomo.zip
var mihomoZip []byte

//go:embed assets/wintun.zip
var wintunZip []byte
