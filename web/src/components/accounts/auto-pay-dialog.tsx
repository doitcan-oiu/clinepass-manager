import { useEffect, useState } from "react"
import { api } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

export function AutoPayDialog({
  open,
  title,
  description,
  pending,
  onOpenChange,
  onConfirm,
}: {
  open: boolean
  title: string
  description: string
  pending?: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (autoPay: boolean) => void
}) {
  const [autoPay, setAutoPay] = useState(false)
  const [configured, setConfigured] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    if (!open) return
    let active = true
    setAutoPay(false)
    setConfigured(false)
    setLoading(true)
    setError("")
    api
      .autoConfig()
      .then((cfg) => { if (active) setConfigured(!!cfg.amzkeys_configured) })
      .catch((err) => { if (active) setError(err instanceof Error ? err.message : "无法读取 Auto 配置") })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [open])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="flex items-start gap-3 rounded-lg border p-3">
          <Checkbox
            id="auto-pay"
            checked={autoPay}
            disabled={!configured}
            onCheckedChange={(v) => setAutoPay(v === true)}
          />
          <div className="grid gap-1">
            <Label htmlFor="auto-pay">自动用 AmzKeys 虚拟卡支付</Label>
            <p className="text-xs text-muted-foreground">
              {loading ? "正在读取 Auto 服务器的支付设置…" : error ? `Auto 未连接：${error}` : configured
                ? "勾选后立刻只开一张卡（已有或正在开则不再开），登录抽链接同时等待。被拒了才换新卡。"
                : "先在设置 → Auto 连接配置远程服务，再到支付卡台保存卡台设置。"}
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button disabled={pending || loading || !!error} onClick={() => onConfirm(autoPay)}>
            {pending ? "正在提交…" : "开始"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
