import type { Batch } from "@/lib/types"

export function waitingCount(b: Batch) {
  return Math.max(0, b.pending - b.failed)
}

export function batchStatus(b: Batch): {
  label: string
  color: string
  primary: "login" | "download" | "refresh" | "paid"
} {
  const waiting = waitingCount(b)
  if (b.paid_count >= b.total && b.total > 0) {
    return { label: "已付款", color: "bg-violet-600 text-white", primary: "paid" }
  }
  if (b.paid_count > 0) {
    if (b.unpaid_cookie_count > 0) {
      return { label: "部分付款", color: "bg-violet-500 text-white", primary: "paid" }
    }
    if (b.unpaid_pay_count > 0) {
      return { label: "部分付款", color: "bg-violet-500 text-white", primary: "download" }
    }
    return { label: "部分付款", color: "bg-violet-500 text-white", primary: "paid" }
  }
  if (b.failed && b.pay_count === 0 && waiting === 0) {
    return { label: "失败", color: "bg-red-600 text-white", primary: "login" }
  }
  if (b.pay_count === 0) {
    return { label: "待生成", color: "bg-amber-500 text-white", primary: "login" }
  }
  if (b.exported_count < b.pay_count) {
    return { label: "可下载", color: "bg-emerald-600 text-white", primary: "download" }
  }
  return { label: "已下载", color: "bg-sky-600 text-white", primary: "paid" }
}
