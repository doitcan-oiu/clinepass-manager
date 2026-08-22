import { useState } from "react"
import { toast } from "sonner"
import { api } from "@/lib/api"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { type LoginProvider, loginProviderLabel } from "@/lib/login-provider"

function nextBatchName() {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, "0")
  return `批次-${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`
}

export function AddBatchDialog({ onSaved }: { onSaved: () => void }) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(nextBatchName)
  const [text, setText] = useState("")
  const [loginProvider, setLoginProvider] = useState<LoginProvider>("google")
  const [pending, setPending] = useState(false)

  function openChange(v: boolean) {
    setOpen(v)
    if (v) {
      setName(nextBatchName())
      setText("")
      setLoginProvider("google")
    }
  }

  async function onSubmit() {
    setPending(true)
    try {
      const res = await api.createBatch({ name, text, login_provider: loginProvider })
      if (res.errors?.length) toast.warning(`批次已创建，部分账号跳过：${res.errors.join("；")}`)
      else toast.success(`已创建 ${res.batch.name}，共 ${res.batch.total} 个账号`)
      setOpen(false)
      onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "创建失败")
    } finally {
      setPending(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={openChange}>
      <DialogTrigger asChild>
        <Button>导入一批账号</Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>导入一批账号</DialogTitle>
          <DialogDescription>
            {loginProvider === "microsoft"
              ? "微软账号一行一个：邮箱----密码。导入后不会自动登录，下一步再点「生成」提取支付链接。"
              : "谷歌账号一行一个：邮箱----密码----辅助邮箱。导入后不会自动登录，下一步再点「生成」提取支付链接。"}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-2">
            <Label>登录方式</Label>
            <div className="grid grid-cols-2 gap-2">
              <Button type="button" variant={loginProvider === "google" ? "default" : "outline"} onClick={() => setLoginProvider("google")}>
                谷歌
              </Button>
              <Button type="button" variant={loginProvider === "microsoft" ? "default" : "outline"} onClick={() => setLoginProvider("microsoft")}>
                微软
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">本批账号都会按「{loginProviderLabel(loginProvider)}」登录。</p>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="batch-name">批次名</Label>
            <Input id="batch-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="batch-text">账号列表</Label>
            <Textarea
              id="batch-text"
              rows={10}
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder={
                loginProvider === "microsoft"
                  ? "user1@outlook.com----pass\nuser2@outlook.com----pass"
                  : "user1@example.com----pass----backup@example.com\nuser2@example.com----pass----backup@example.com"
              }
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            取消
          </Button>
          <Button disabled={pending || !text.trim()} onClick={onSubmit}>
            导入这批账号
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
