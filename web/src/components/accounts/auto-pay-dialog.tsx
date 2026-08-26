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

  useEffect(() => {
    if (!open) return
    setAutoPay(false)
    api
      .config()
      .then((cfg) => setConfigured(!!cfg.amzkeys_configured))
      .catch(() => setConfigured(false))
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
              {configured
                ? "抽出支付链接后用当前这张虚拟卡付 Stripe。同一张卡会一直用，被拒了才开新卡。"
                : "先到设置 → amzkeys卡台 填好 Host、AppID、AppKey、RSA2 私钥和卡段。"}
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button disabled={pending} onClick={() => onConfirm(autoPay)}>
            开始
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
