# jump 演示录屏脚本 — vhs docs/demo.tape
# 安全规则（血泪教训）：
#   1. 过滤必须先按 `/` 进过滤态，否则按键会落在普通态快捷键上（d=删MRU）
#   2. 非过滤态的 Enter = 选中并 exec ssh，全程禁止；只按 q 退出
#   3. host 名全部 demo- 前缀，即使误触 Enter 也只会 resolve 失败，碰不到真机器
#   4. 设置面板只按 esc 回滚，不按 enter（避免写真实配置文件）
# JUMP_THEME=default 锁经典紫，和 README 截图一致

Output docs/demo.mp4

Set FontSize 22
Set Width 1280
Set Height 760
Set TypingSpeed 60ms
Set Theme "Catppuccin Mocha"

# 1. 全列表 → `/` 进过滤态 → 模糊过滤 prod（tag 命中）→ 回车确认过滤
Type "SSH_CONFIG=docs/demo-ssh-config JUMP_THEME=default jump"
Sleep 500ms
Enter
Sleep 2s
Type "/"
Sleep 400ms
Type "prod"
Sleep 1.5s
Enter
Sleep 1.2s
Type "q"
Sleep 800ms

# 2. svc 也参与过滤
Type "SSH_CONFIG=docs/demo-ssh-config JUMP_THEME=default jump"
Sleep 300ms
Enter
Sleep 1.5s
Type "/"
Sleep 400ms
Type "grafana"
Sleep 1.5s
Enter
Sleep 1s
Type "q"
Sleep 800ms

# 3. t 打开设置面板，翻主题实时预览，esc 回滚不保存
Type "SSH_CONFIG=docs/demo-ssh-config JUMP_THEME=default jump"
Sleep 300ms
Enter
Sleep 1.5s
Type "t"
Sleep 1.2s
Down
Sleep 900ms
Down
Sleep 900ms
Down
Sleep 900ms
Down
Sleep 1s
Escape
Sleep 1s
Type "q"
Sleep 800ms
