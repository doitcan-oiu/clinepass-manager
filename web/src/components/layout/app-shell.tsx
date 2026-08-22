import { Outlet } from "react-router-dom"
import { AppHeader } from "@/components/layout/app-header"

export function AppShell() {
  return (
    <div className="min-h-svh bg-background">
      <AppHeader />
      <main className="mx-auto w-full max-w-[96rem] flex-1 p-4 md:p-6">
        <Outlet />
      </main>
    </div>
  )
}
