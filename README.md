# 梦盒 Windows 版 v0.10.0

桌面壳 + mihomo 内核的 TUN 全局代理客户端，绿色单文件，解压即用。
原生 WebView2 应用窗口（不是浏览器标签页），系统托盘常驻；
点 X 默认最小化到托盘（设置可改成直接退出），退出走托盘右键菜单。

## 使用方法

1. **右键「以管理员身份运行」梦盒.exe**（TUN 全局模式必需）
2. 自动弹出「梦盒」原生窗口（内嵌控制台）
3. 在「订阅」页粘贴订阅链接（支持 Clash YAML 与 sing-box JSON），点「添加」
   ；或点「➕ 添加节点」粘贴 vless / vmess / trojan / ss / hysteria2 单链接
4. 复制代理链接到剪贴板后，会自动弹出导入提示（可导入/忽略）
5. 在「节点」页点「⚡ 全部测速」，再点节点切换
6. 需要代理的软件（浏览器、游戏、纸飞机等）即可走代理
7. 托盘图标右键：显示主界面 / TUN 开关 / 重启内核 / 退出

## 功能

- 📈 流量统计：左侧导航「统计」页；今日/本周分段切换，柱状图（今日 24 小时 /\n  本周 7 天），总量（上行/下行），目标域名 Top 排行（按连接目标聚合，Top 20）；\n  小时粒度落盘 `%APPDATA%\\DreamBox\\traffic_history.json`，保留 90 天；\n  历史数据从本版本开始累计，升级前无数据；\n  注：Windows TUN 模式下系统层面流量全部归属 mihomo 进程，无法按应用细分
- 🛰️ 节点列表 / 切换 / 延迟测速（vless / vmess / trojan / ss / hysteria2）
- 🔗 订阅管理：增删改、手动更新（Clash YAML / sing-box JSON 自动识别）；
  支持 `subscription-userinfo` 响应头，订阅卡片显示流量用量进度条（已用/总量/百分比）与到期日期
- 📊 专业面板三选一：左侧导航「面板」进入专业面板（节点分组、连接拓扑、
  延迟柱状图、规则管理等），左上角「← 返回梦盒」悬浮按钮可返回主界面；
  设置页「面板」卡片可切换：Meta 面板 / Yacd 经典 / Zashboard，切换后自动
  释放并重启内核生效。首次启动自动释放到 `%APPDATA%\DreamBox\panel\`
  - Meta 面板：[metacubexd](https://github.com/MetaCubeX/metacubexd) v1.273.1，
    MIT License，Copyright (c) 2023 MetaCubeX
  - Yacd 经典：[Yacd-meta](https://github.com/MetaCubeX/Yacd-meta)（yacd 的 Meta 内核分支，
    gh-pages 构建 2026-10-05；上游未附 LICENSE，原 yacd 为 MIT 许可，作者 hacdias）
  - Zashboard：[zashboard](https://github.com/Zephyruso/zashboard) v3.29.1，
    MIT License，Copyright 2024 Zephyruso
- ➕ 手动节点：粘贴单链接 → 解析预览（名称/类型/地址:端口）→ 确认添加；
  节点列表中带「手动」角标；与订阅重名时手动节点优先
- 📋 剪贴板自动导入：复制代理链接后自动弹窗询问是否导入（Windows）
- 🌐 TUN 全局模式：接管整机流量，游戏、微软商店应用、终端命令都走代理
- 🔌 系统代理开关：127.0.0.1:7890（浏览器等走系统代理的软件）
- 🚀 开机自启（后台运行，不弹窗口；「开机自启时不显示窗口」可关，关闭后自启会弹出主窗口）
- 🗂️ 系统托盘：点 X 最小化到托盘（默认开，设置可改）；左键单击托盘图标显示主界面；
  右键菜单：显示主界面 / TUN 开启（勾选态） / 重启内核 / 退出；悬浮提示显示运行状态
- 🔁 重启内核：首页内核状态卡片「重启」按钮，或托盘右键「重启内核」
- 📈 流量总计：首页实时流量卡片下方显示自启动以来的总上传/总下载
- 🌍 出口 IP 归属地/ISP：出口 IP 卡片显示国家与 ISP（ip-api 查询）；眼睛图标可切换 IP 明文/掩码显示（如 38.209.*.*）
- 🔄 自动检查更新：设置里开启并填写 version.json 地址，启动后有新版本首页横幅提示
- 📝 订阅 User-Agent 可配：设置里修改订阅下载时使用的 UA（默认 Chrome）
- 📊 实时流量曲线、活跃连接数、出口 IP 检测
- 出站模式：规则 / 全局 / 直连
- 🎨 主题系统：「设置 → 主题」可切换、上传（zip）、删除主题；
  内置「默认深色」与「波奇酱·星空」（粉色星空，纯 CSS 实现）；
  主题目录 `%APPDATA%\DreamBox\themes\<id>\`（theme.json + theme.css）
- ✨ 启动器动画：启动时梦字 logo 呼吸光晕 + 流光环，状态分阶段显示，内核就绪后淡出

## 文件说明

- `梦盒.exe`：主程序（内嵌 mihomo 内核 v1.19.32 与 wintun 驱动，首次运行自动释放）
- 用户数据存放在 `%APPDATA%\DreamBox\`（配置、订阅、手动节点 manual.json、内核）

## 注意事项

- **必须以管理员身份运行**，否则 TUN 无法创建虚拟网卡；非管理员运行时会弹窗提示
- 部分杀毒软件可能误报或拦截 TUN 驱动（wintun），如遇拦截请加白名单
- TUN 与其他 VPN 软件冲突，请勿同时开启
- 首次启动 Windows 防火墙可能弹窗，点「允许访问」
- 本机端口占用：7890（代理）、9090（内核控制）、17890（控制台），如冲突请先关闭占用软件
- 点 X 默认最小化到托盘（内核继续跑），设置里可改成直接退出；
  托盘初始化失败时自动按退出处理；真正退出请右键托盘图标 → 退出
- 开机自启时以后台方式运行

## 源码

Go 编写（`CGO_ENABLED=0 GOOS=windows go build` 交叉编译，WebView2 窗口纯 Go 实现），详见源码目录。

## 第三方组件与许可

- [mihomo](https://github.com/MetaCubeX/mihomo) v1.19.32（内核，内嵌二进制首次运行释放）：GPL-3.0，Copyright MetaCubeX
- [wintun](https://www.wintun.net/) 0.14.1（TUN 驱动，内嵌 dll 首次运行释放）：Wintun 许可，Copyright (c) WireGuard LLC
- [metacubexd](https://github.com/MetaCubeX/metacubexd) v1.273.1（专业面板其一）：MIT，Copyright (c) 2023 MetaCubeX
- [zashboard](https://github.com/Zephyruso/zashboard) v3.29.1（专业面板其一）：MIT，Copyright 2024 Zephyruso
- [Yacd-meta](https://github.com/MetaCubeX/Yacd-meta)（专业面板其一，yacd 的 Meta 内核分支）：上游未附 LICENSE 文件，原 yacd 为 MIT 许可，作者 hacdias

本项目源码采用 MIT 许可；mihomo 内核为 GPL-3.0，分发二进制时请遵守其许可条款。
