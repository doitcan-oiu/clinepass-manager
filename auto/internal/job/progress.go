package job

import (
	"strings"

	"opencode-go-manager/auto/internal/login"
	"opencode-go-manager/internal/model"
)

const (
	StageQueued       = "queued"
	StageBrowser      = "browser"
	StageLogin        = "login"
	StageVerification = "verification"
	StagePayment      = "payment"
	StageSaving       = "saving"
	StageDone         = "done"
	maxJobLogs        = 300
)

func stageLabel(stage string) string {
	switch stage {
	case StageQueued:
		return "等待执行"
	case StageBrowser:
		return "启动浏览器"
	case StageLogin:
		return "账号登录"
	case StageVerification:
		return "安全验证"
	case StagePayment:
		return "提取与支付"
	case StageSaving:
		return "保存结果"
	case StageDone:
		return "已完成"
	}
	return ""
}

func setStageLocked(job *model.Job, stage string, now int64) {
	if stage == "" || stage == job.Stage {
		return
	}
	job.Stage = stage
	job.StageLabel = stageLabel(stage)
	job.StageStartedAt = now
}

func containsAny(s string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(s, candidate) {
			return true
		}
	}
	return false
}

// presentLog keeps the existing worker protocol usable while exposing stable
// progress stages. Browser cleanup and configuration never move progress.
func presentLog(raw, level string) (message, detail, eventLevel, stage string) {
	message = login.CompactMessage(raw)
	eventLevel = level
	lower := strings.ToLower(message)
	technical := containsAny(lower, "登录引擎=", "无头模式=", "登录方式=", "使用全局代理", "未设代理", "geoip", "cloak 官方包装", "用户 id:", "自动支付=", "释放 cloak", "关闭浏览器", "已注入", "已保存", "已更新", "点击后 url=", "到达 authkit，")
	if technical && level != "error" && level != "warn" {
		eventLevel = "debug"
	} else {
		switch {
		case strings.HasPrefix(message, "正在保存"):
			stage = StageSaving
		case strings.HasPrefix(message, "开始登录"), strings.HasPrefix(message, "开始刷新"), strings.HasPrefix(message, "开始用浏览器"), containsAny(message, "启动浏览器", "浏览器已启动"):
			stage = StageBrowser
		case containsAny(lower, "支付", "stripe", "账单", "虚拟卡", "结算货币", "3ds"):
			stage = StagePayment
		case containsAny(lower, "验证码", "手机", "接码", "验证页", "安全检查", "安全验证", "辅助邮箱", "hero sms", "radar", "人工验证"):
			stage = StageVerification
		case containsAny(lower, "microsoft", "微软", "google", "谷歌", "登录", "授权", "authkit", "邀请链接", "进入 cline", "填写密码", "账号下一步", "密码下一步", "邮箱已提交", "密码已提交"):
			stage = StageLogin
		}
	}
	// A routine close must not conceal a real failure just because the error
	// mentions the browser; error logs remain visible and preserve the stage.
	if technical || level == "error" || message == "登录完成" || message == "刷新完成" || message == "自动支付成功" {
		stage = ""
	}
	for _, marker := range []string{"，当前 URL=", "当前 URL=", " URL="} {
		if i := strings.Index(message, marker); i > 0 {
			detail = login.CompactMessage(strings.TrimSpace(message[i+len(marker):]))
			message = strings.TrimSpace(message[:i])
			break
		}
	}
	if strings.HasPrefix(message, "支付链接:") || strings.HasPrefix(message, "支付链接：") {
		message = "支付链接已提取"
		// The real link is available on the account. Do not repeat a checkout
		// capability URL in its progress feed.
		detail = ""
	}
	return
}
