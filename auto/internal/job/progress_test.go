package job

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"opencode-go-manager/internal/config"
	"opencode-go-manager/internal/model"
	"opencode-go-manager/internal/store"
)

func progressManager() (*Manager, *model.Job, *model.Job) {
	m := New(config.Config{}, nil)
	a := &model.Job{ID: "first-job", AccountID: "first-account", Status: "queued", Stage: StageQueued, StageLabel: stageLabel(StageQueued), StageStartedAt: 1}
	b := &model.Job{ID: "second-job", AccountID: "second-account", Status: "queued", Stage: StageQueued, StageLabel: stageLabel(StageQueued), StageStartedAt: 1}
	m.jobs[a.ID] = a
	m.jobs[b.ID] = b
	return m, a, b
}

func TestRepeatedLogsFoldPerAccountWithStableSequence(t *testing.T) {
	m, a, b := progressManager()
	m.setStatus(a, "running", "")
	m.setStatus(b, "running", "")
	ch, cancel := m.Subscribe(a.ID)
	defer cancel()
	for i := 0; i < 3; i++ {
		m.logf(a, "info", "微软密码卡片尚未就绪，当前 URL=https://login.live.com/login?state=secret-%d", i)
		m.logf(b, "info", "填写 Google 账号")
	}
	if len(a.Logs) != 1 || a.Logs[0].Repeat != 3 || a.Logs[0].Sequence != 1 || a.Logs[0].AccountID != a.AccountID {
		t.Fatalf("first account: %+v", a.Logs)
	}
	if len(b.Logs) != 1 || b.Logs[0].Repeat != 3 || b.Logs[0].Sequence != 1 || b.Logs[0].AccountID != b.AccountID {
		t.Fatalf("second account: %+v", b.Logs)
	}
	for i := 1; i <= 3; i++ {
		ev := <-ch
		if ev.AccountID != a.AccountID || ev.Repeat != i || ev.Sequence != 1 {
			t.Fatalf("subscriber update %d: %+v", i, ev)
		}
	}
	if a.Logs[0].Detail != "https://login.live.com/login" || strings.Contains(a.Logs[0].Message, "URL") {
		t.Fatalf("noisy or sensitive event: %+v", a.Logs[0])
	}
	m.logf(a, "info", "填写 Microsoft 密码")
	if a.Logs[1].Sequence != 2 || a.Logs[1].Repeat != 1 {
		t.Fatalf("next event: %+v", a.Logs[1])
	}
}

func TestJobStagesAndFailurePosition(t *testing.T) {
	m, a, _ := progressManager()
	if a.StartedAt != 0 {
		t.Fatal("queued job must not have a running time")
	}
	m.setStatus(a, "running", "")
	if a.StartedAt < time.Now().Unix()-1 || a.Stage != StageBrowser {
		t.Fatalf("job did not start: %+v", a)
	}
	m.logf(a, "info", "填写 Microsoft 密码")
	if a.Stage != StageLogin {
		t.Fatalf("login stage: %+v", a)
	}
	a.StageStartedAt = 123
	m.logf(a, "info", "微软密码卡片尚未就绪，当前 URL=https://login.live.com/login?state=secret")
	if a.StageStartedAt != 123 {
		t.Fatal("same stage reset its elapsed time")
	}
	m.logf(a, "info", "正在正常关闭浏览器，释放 Cloak 会话")
	if a.Stage != StageLogin || a.Logs[len(a.Logs)-1].Level != "debug" {
		t.Fatalf("cleanup replaced failure stage: %+v", a)
	}
	m.logf(a, "error", "密码提交后页面没有变化，请检查登录页面")
	m.setStatus(a, "failed", "Microsoft 登录未完成，当前 URL=https://login.live.com/login?state=secret")
	if a.Stage != StageLogin || a.EndedAt < a.StartedAt || strings.Contains(a.Error, "secret") {
		t.Fatalf("wrong failure state: %+v", a)
	}
	m.logf(a, "info", "等待短信验证码")
	if a.Stage != StageLogin {
		t.Fatal("late worker log changed a finished job's stage")
	}
}

func TestVerificationPaymentAndSavingStages(t *testing.T) {
	m, a, _ := progressManager()
	m.setStatus(a, "running", "")
	for _, tc := range []struct{ message, stage string }{
		{"打开邀请链接 https://authkit.cline.bot", StageLogin},
		{"填写辅助邮箱", StageVerification},
		{"等待短信验证码", StageVerification},
		{"支付链接: https://checkout.stripe.com/pay/private?token=secret", StagePayment},
		{"填写 3DS 验证码", StagePayment},
		{"正在保存账号和支付结果", StageSaving},
	} {
		m.logf(a, "info", "%s", tc.message)
		if a.Stage != tc.stage || a.StageLabel != stageLabel(tc.stage) || a.StageStartedAt == 0 {
			t.Fatalf("%s: %+v", tc.message, a)
		}
	}
	for _, ev := range a.Logs {
		if strings.Contains(ev.Message+ev.Detail, "private") || strings.Contains(ev.Message+ev.Detail, "secret") {
			t.Fatalf("payment link leaked in progress: %+v", ev)
		}
	}
	m.setStatus(a, "success", "")
	if a.Stage != StageDone || a.EndedAt < a.StartedAt {
		t.Fatalf("not done: %+v", a)
	}
}

func TestFailureAndTechnicalLogsDoNotMovePaymentStage(t *testing.T) {
	m, a, _ := progressManager()
	m.setStatus(a, "running", "")
	m.logf(a, "info", "打开 Stripe 支付页")
	for _, tc := range []struct{ level, message string }{
		{"error", "支付失败：请重新进行 Microsoft 登录"},
		{"info", "正在正常关闭浏览器，释放 Cloak 会话"},
		{"info", "登录引擎=python cloak humanize"},
	} {
		m.logf(a, tc.level, "%s", tc.message)
		if a.Stage != StagePayment {
			t.Fatalf("%s replaced payment stage with %s", tc.message, a.Stage)
		}
	}
	if a.Logs[1].Level != "error" {
		t.Fatal("real failure was hidden in debug logs")
	}
}

func TestJobSnapshotsOwnLogsAndHistoryIsBounded(t *testing.T) {
	m, a, _ := progressManager()
	m.logf(a, "info", "等待短信验证码")
	snapshot, _ := m.Get(a.ID)
	list := m.List()
	m.logf(a, "info", "等待短信验证码")
	if snapshot.Logs[0].Repeat != 1 {
		t.Fatal("Get snapshot shared mutable logs")
	}
	for _, item := range list {
		if item.ID == a.ID && item.Logs[0].Repeat != 1 {
			t.Fatal("List snapshot shared mutable logs")
		}
	}
	snapshot.Logs[0].Message = "mutated externally"
	if a.Logs[0].Message == snapshot.Logs[0].Message {
		t.Fatal("external snapshot changed manager")
	}
	for i := 0; i < maxJobLogs+30; i++ {
		m.logf(a, "info", "阶段消息 %d", i)
	}
	if len(a.Logs) != maxJobLogs || a.Logs[len(a.Logs)-1].Sequence != maxJobLogs+31 {
		t.Fatalf("bad history bound or sequence: len=%d last=%+v", len(a.Logs), a.Logs[len(a.Logs)-1])
	}
}

func TestSubscribeCancellationWhileLogging(t *testing.T) {
	m, a, _ := progressManager()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			m.logf(a, "info", "进度 %d", i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			_, cancel := m.Subscribe(a.ID)
			cancel()
			cancel()
			m.Get(a.ID)
			m.List()
		}
	}()
	wg.Wait()
}

func TestEnqueueReturnsDetachedQueuedSnapshot(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	acc, err := st.CreatePaidAccount(model.CreatePaidAccountInput{Email: "queued@example.com", CookieHeader: "sid=test"})
	if err != nil {
		t.Fatal(err)
	}
	m := New(config.Config{}, st)
	m.HoldPump()
	snapshot, err := m.EnqueueCookie(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Stage != StageQueued || snapshot.StartedAt != 0 || snapshot.StageStartedAt == 0 {
		t.Fatalf("bad queued progress: %+v", snapshot)
	}
	m.logf(m.jobs[snapshot.ID], "info", "准备启动浏览器")
	if len(snapshot.Logs) != 0 || snapshot.Stage != StageQueued {
		t.Fatalf("enqueue snapshot changed: %+v", snapshot)
	}
}
