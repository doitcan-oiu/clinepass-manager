import { BrowserRouter, Route, Routes } from "react-router-dom"
import { ThemeProvider } from "next-themes"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import { AppShell } from "@/components/layout/app-shell"
import { DashboardPage } from "@/pages/dashboard"
import { AutomationPage } from "@/pages/automation"
import { BatchDetailPage } from "@/pages/batch-detail"
import { AccountsPage } from "@/pages/accounts"
import { LogsPage } from "@/pages/logs"
import { SettingsPage } from "@/pages/settings"

export default function App() {
  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
      <TooltipProvider>
        <BrowserRouter>
          <Routes>
            <Route element={<AppShell />}>
              <Route path="/" element={<DashboardPage />} />
              <Route path="/automation" element={<AutomationPage />} />
              <Route path="/automation/:id" element={<BatchDetailPage />} />
              <Route path="/account" element={<AccountsPage />} />
              <Route path="/logs" element={<LogsPage />} />
              <Route path="/settings" element={<SettingsPage />} />
            </Route>
          </Routes>
        </BrowserRouter>
        <Toaster />
      </TooltipProvider>
    </ThemeProvider>
  )
}
