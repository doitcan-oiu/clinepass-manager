import { useEffect, useState } from "react"
import { toast } from "sonner"
import { api } from "@/lib/api"
import type { Account } from "@/lib/types"
import { statusLabel, statusVariant } from "@/lib/status"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { loginProviderLabel } from "@/lib/login-provider"

async function copy(value?: string) {
  if (!value) return
  await navigator.clipboard.writeText(value)
  toast.success("已复制")
}

function Field({ label, value }: { label: string; value?: string }) {
  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between">
        <Label>{label}</Label>
        {value ? (
          <Button type="button" variant="ghost" size="xs" onClick={() => copy(value)}>
            复制
          </Button>
        ) : null}
      </div>
      <Input readOnly value={value || "—"} className="font-mono text-xs" />
    </div>
  )
}

export function DetailDialog({
  account,
  open,
  onOpenChange,
}: {
  account: Account | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [full, setFull] = useState<Account | null>(null)

  useEffect(() => {
    if (!open || !account) {
      setFull(null)
      return
    }
    let cancelled = false
    api
      .account(account.id)
      .then((a) => {
        if (!cancelled) setFull(a)
      })
      .catch((e) => {
        if (!cancelled) toast.error(e instanceof Error ? e.message : "加载详情失败")
      })
    return () => {
      cancelled = true
    }
  }, [open, account?.id])

  const view = full || account
  if (!view) return null
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{view.email}</DialogTitle>
          <DialogDescription>
            <Badge variant={statusVariant(view.status)}>{statusLabel(view.status)}</Badge>
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <Field label="登录方式" value={loginProviderLabel(view.login_provider)} />
          <Field label="userID" value={view.user_id || view.workspace_id} />
          <Field label="key" value={view.api_key} />
          <Field label="pay" value={view.payment_url} />
          <div className="grid gap-2">
            <Label>cookie</Label>
            <Textarea readOnly rows={4} value={view.cookie_header || "—"} className="max-h-32 font-mono text-xs" />
          </div>
          <div className="grid gap-2">
            <Label>cookies.json</Label>
            <Textarea readOnly rows={6} value={view.cookies_json || "—"} className="max-h-40 font-mono text-xs" />
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
