import assert from "node:assert/strict"
import test from "node:test"
import {
  batchRunCounts, displayEvents, elapsedLabel, latestJobsByAccount, logsFromJobs,
  safeLogText, shortLogText, stageLabel,
} from "../src/lib/job-log.ts"

const event = (overrides = {}) => ({ job_id: "job-1", account_id: "a", time: 100000, level: "info", message: "正在登录", ...overrides })
const job = (overrides = {}) => ({ id: "job-1", account_id: "a", status: "running", started_at: 100, logs: [event()], ...overrides })

test("old OAuth logs never expose query, fragment, URL credentials or credential fields", () => {
  const source = "打开 https://alice:private@login.live.com/oauth?code=secret&state=private#token password=private\nAuthorization: Bearer private\nCookie: session=private; other=private"
  const text = safeLogText(source)
  assert.ok(text.includes("https://login.live.com/oauth"))
  assert.doesNotMatch(text, /private|secret|token|alice|Bearer|session=/)
  assert.equal(shortLogText("Microsoft 登录未完成，当前 URL=https://login.live.com/oauth?code=private"), "Microsoft 登录未完成")
})

test("latest task is selected by time, never by old running status", () => {
  const old = job({ started_at: 100 })
  const current = job({ id: "job-2", started_at: 200, status: "failed", logs: [event({ job_id: "job-2", time: 200001 })] })
  for (const jobs of [[old, current], [current, old]]) assert.equal(latestJobsByAccount(jobs).a.id, "job-2")
})

test("new queued task with no started_at replaces completed history, including same second", () => {
  const old = job({ started_at: 200, status: "failed", logs: [event({ time: 200010 })] })
  const current = job({ id: "new", status: "queued", started_at: 0, stage_started_at: 200, logs: [] })
  for (const jobs of [[old, current], [current, old]]) assert.equal(latestJobsByAccount(jobs).a.id, "new")
})

test("same-second task comparison uses first event milliseconds", () => {
  const old = job({ started_at: 200, logs: [event({ time: 200010 })] })
  const current = job({ id: "new", started_at: 200, logs: [event({ time: 200900 })] })
  assert.equal(latestJobsByAccount([current, old]).a.id, "new")
})

test("snapshot replacement includes updated repeat counts and excludes prior attempts", () => {
  const old = job({ started_at: 50, logs: [event({ message: "历史失败" })] })
  const current = job({ id: "new", logs: [event({ job_id: "new", sequence: 1, repeat: 1 })] })
  assert.equal(displayEvents(logsFromJobs([old, current]))[0].repeat, 1)
  current.logs[0].repeat = 7
  const logs = displayEvents(logsFromJobs([old, current]))
  assert.equal(logs.length, 1)
  assert.equal(logs[0].repeat, 7)
})

test("consecutive repeats merge per account while preserving stages and different diagnoses", () => {
  const logs = displayEvents([
    event({ stage: "login", repeat: 3, detail: "page A" }),
    event({ account_id: "b", message: "其他账号" }),
    event({ stage: "login", time: 101000, detail: "page A" }),
    event({ stage: "login", time: 102000, detail: "page B" }),
    event({ stage: "verification", time: 103000, detail: "page B" }),
  ])
  assert.equal(logs.length, 4)
  assert.equal(logs[0].repeat, 4)
  assert.equal(logs[0].lastTime, 101000)
})

test("technical filtering never suppresses errors or warnings", () => {
  const logs = [event({ level: "debug", message: "debug" }), event({ message: "正在正常关闭浏览器，释放 Cloak 会话" }), event({ level: "error", message: "当前 URL=https://example.com?secret=private" }), event({ level: "warning", message: "请完成身份验证" })]
  assert.equal(displayEvents(logs).length, 2)
  assert.equal(displayEvents(logs, true).length, 4)
  assert.doesNotMatch(displayEvents(logs)[0].message, /private/)
})

test("batch totals count every account once and queued accounts need no logs", () => {
  const accounts = [
    { id: "a", status: "failed" }, { id: "b", status: "pending" },
    { id: "c", status: "ready" }, { id: "d", status: "failed" },
  ]
  assert.deepEqual(batchRunCounts(accounts, { a: job() }), { queued: 1, running: 1, success: 1, failed: 1 })
})

test("elapsed time freezes after completion and uses real stage time", () => {
  assert.equal(elapsedLabel(job({ ended_at: 175 }), 900000), "1 分 15 秒")
  assert.equal(elapsedLabel(job({ stage_started_at: 140 }), 145000, true), "5 秒")
  assert.equal(elapsedLabel(job({ started_at: 0, status: "queued" })), "—")
  assert.equal(stageLabel(job({ status: "failed" })), "任务未完成")
})
