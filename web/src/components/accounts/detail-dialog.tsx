import { toast } from "sonner"
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
  if (!account) return null
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{account.email}</DialogTitle>
          <DialogDescription>
            <Badge variant={statusVariant(account.status)}>{statusLabel(account.status)}</Badge>
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <Field label="登录方式" value={loginProviderLabel(account.login_provider)} />
          <Field label="userID" value={account.user_id || account.workspace_id} />
          <Field label="key" value={account.api_key} />
          <Field label="pay" value={account.payment_url} />
          <div className="grid gap-2">
            <Label>cookie</Label>
            <Textarea readOnly rows={4} value={account.cookie_header || "—"} className="max-h-32 font-mono text-xs" />
          </div>
          <div className="grid gap-2">
            <Label>cookies.json</Label>
            <Textarea readOnly rows={6} value={account.cookies_json || "—"} className="max-h-40 font-mono text-xs" />
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
